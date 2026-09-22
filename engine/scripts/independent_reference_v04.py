#!/usr/bin/env python3
"""Independent stdlib-Python arithmetic check of saved NEW Go outputs.

Uses observed initial values and saved forecast forcing, NOT Go libraries.
It is an engineering cross-check of the declared model, not external validation.
"""
from __future__ import annotations
import argparse, collections, gzip, hashlib, json, math
from pathlib import Path

MAX_ERROR = 0.0
COMPARISONS = 0

def same(a: float, b: float, what: str = "numeric") -> None:
    global MAX_ERROR, COMPARISONS
    assert math.isfinite(a) and math.isfinite(b), what
    error = abs(a-b)
    MAX_ERROR = max(MAX_ERROR, error)
    COMPARISONS += 1
    assert error <= 2e-10*max(1,abs(a),abs(b)), (what,a,b,error)

def load(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))

def rows(path: Path):
    with gzip.open(path,"rt",encoding="utf-8") as f:
        for line in f:
            yield json.loads(line)

def valid(m):
    return m["present"] and m.get("value") is not None and not m.get("fault") and math.isfinite(m["value"]) and m["value"]>=0

def features(packet):
    rr=packet["readings"]
    fields=(("demand_a","demand_b"),("regular_raw_arrival",),("capacity_before_overtime",))
    thresholds=(71.5,76.5,85.0)
    out=[]
    for j,fs in enumerate(fields):
        if not all(valid(rr[f]) and rr[f]["age_days"]==0 for f in fs):
            out.append(None);continue
        v=sum(rr[f]["value"] for f in fs)
        out.append(v>thresholds[j] if j==0 else v<thresholds[j])
    return out

def quality(packet):
    rr=packet["readings"]
    present=sum(m["present"] for m in rr.values())
    fresh=sum(m["present"] and m["age_days"]==0 for m in rr.values())
    passed=sum(valid(m) for m in rr.values())
    def check(fs, fn):
        return all(valid(rr[f]) for f in fs) and fn(*[rr[f]["value"] for f in fs])
    ot=packet["context"]["executed_overtime"]
    passed+=check(("output_a","output_b","capacity_before_overtime"),lambda a,b,c:a+1.6*b<=1.2*c*(1+ot))
    passed+=check(("shipped_a","demand_a","backlog_before_a"),lambda s,d,b:s<=1.2*(d+b))
    passed+=check(("shipped_b","demand_b","backlog_before_b"),lambda s,d,b:s<=1.2*(d+b))
    passed+=check(("capacity_before_overtime",),lambda c:c<=120)
    return .4*present/15+.3*(fresh/present if present else 0)+.3*passed/19

def check_fit(base):
    setup=load(base/"setup.json");summary=load(base/"summary.json");model=load(base/"diagnostic_model.json")
    specs={s["episode_id"]:s for s in setup["training"]}
    train_seeds={s["seed"] for s in setup["training"]}
    validation_seeds={s["seed"] for s in setup["development_validation_not_final_holdout"]}
    assert train_seeds.isdisjoint(validation_seeds)
    n=collections.Counter();ones=collections.defaultdict(lambda:[0,0,0]);retained=[];seen=set();nr=0
    for audit in rows(base/"training_audit.jsonl.gz"):
        r=audit["synthetic_source"];ep=specs[r["episode_id"]];day=r["day_zero_based"]
        assert (r["episode_id"],day) not in seen;seen.add((r["episode_id"],day));nr+=1
        evt=ep["event"];active=evt["start_day_zero_based"]<=day<=evt["end_day_zero_based"]
        assert active==r["introduced_cause_active"] and r["calendar_day_one_based"]==day+1
        assert r["experiment_label_not_inference_input"]==evt["label"]
        f=features(r["observation"]);q=quality(r["observation"])
        same(q,audit["quality"]["score"],"quality")
        assert f==[x["value"] for x in audit["features"]]
        use=active and q>=.7 and all(x is not None for x in f)
        assert use==audit["included_in_fit"]
        if use:
            label=evt["label"];n[label]+=1
            for j,v in enumerate(f):ones[label][j]+=v
            retained.append({"id":r["id"],"label":label,"features":f})
    assert nr==sum(s["days"] for s in specs.values())==summary["training_days"]
    retained.sort(key=lambda x:x["id"])
    assert retained==load(base/"retained_examples.json")
    digest=hashlib.sha256(json.dumps(retained,separators=(",",":"),ensure_ascii=False).encode()).hexdigest()
    assert digest==model["provenance"]["dataset_sha256"]
    for h in model["hypotheses"]:
        for j,p in enumerate(h["probability_feature_present"]):same(p,(ones[h["id"]][j]+1)/(n[h["id"]]+2),"Laplace fit")
        assert h["prior"]==.25
    counts=collections.defaultdict(lambda:collections.Counter());failures=[];vr=0
    for audit in rows(base/"validation_audit.jsonl.gz"):
        vr+=1;r=audit["synthetic_source"];label=r["experiment_label_not_inference_input"]
        mon=audit["analysis"]["monitor"];diag=audit["analysis"]["diagnosis"];f=features(r["observation"])
        active=r["introduced_cause_active"];c=counts[label];c["active"]+=active
        eligible=bool(active and mon["computed"] and mon["event"] and all(x is not None for x in f))
        assert eligible==audit["eligible_for_conditional_accuracy"]
        if eligible:
            c["evaluated"]+=1;rank=[x["id"] for x in diag["ranked"]]
            c["top1"]+=bool(rank and rank[0]==label)
            if label not in diag["candidate_ids"]:
                c["excluded_by_graph"]+=1
                failures.append({"id":r["id"],"label":label,"features":f,"candidates":diag["candidate_ids"],"signals":mon["signals"]})
    assert vr==summary["validation_days"]
    for label,c in counts.items():
        s=summary["development_validation_counts"][label]
        assert c["active"]==s["active_days"] and c["evaluated"]==s["eligible_diagnostic_days"] and c["top1"]==s["correct_top1"]
    return {"training_rows_checked":nr,"validation_rows_checked":vr,"training_examples_checked":len(retained),"training_dataset_sha256":digest,"counts":dict(counts),"graph_exclusion_cases":failures}

def step(state,forcing,action,h,par):
    raw,finished,backlog,baseplan=state
    plan=list(baseplan)
    if h==0 and action["permanent_plan"] is not None:plan=list(action["permanent_plan"])
    factors=action["temporary_factors"] if h==0 else [1,1]
    executed=[plan[i]*factors[i] for i in range(2)]
    cap=forcing["capacity"]*(1+(action["overtime"] if h==0 else 0))
    available=raw+forcing["raw_arrival"]+(action["extra_raw"] if h==0 else 0)
    cp=par["capacity_per_unit"];rp=par["raw_per_unit"]
    needcap=sum(cp[i]*executed[i] for i in range(2));needraw=sum(rp[i]*executed[i] for i in range(2))
    scale=min(1,cap/needcap if needcap else 1,available/needraw if needraw else 1)
    produced=[scale*p for p in executed]
    demand=[forcing["demand"][i]+backlog[i] for i in range(2)]
    stock=[finished[i]+produced[i] for i in range(2)]
    shipped=[min(stock[i],demand[i]) for i in range(2)]
    nb=[demand[i]-shipped[i] for i in range(2)];nf=[stock[i]-shipped[i] for i in range(2)]
    nr=max(0,available-sum(rp[i]*produced[i] for i in range(2)))
    k=[sum(shipped)/sum(demand) if sum(demand) else 1,
       max(0,1-sum(abs(produced[i]-executed[i]) for i in range(2))/sum(executed)) if sum(executed) else 1,
       sum(cp[i]*produced[i] for i in range(2))/cap if cap else 0,
       nr/par["raw_days_denominator"]]
    lo=[.95,.95,.7,3];hi=[1,1,.9,7];weights=[.35,.25,.15,.25];above=[0,0,.5,.25]
    loss=sum(weights[j]*(max(lo[j]-k[j],0)+above[j]*max(k[j]-hi[j],0))/lo[j] for j in range(4))
    return (nr,nf,nb,plan),k,loss

def quantile(v,p,tau):
    mass=sum(p);atoms=sorted((x,w/mass) for x,w in zip(v,p) if w>0);s=0
    for x,w in atoms:
        s+=w
        if s>=tau:return x
    return atoms[-1][0]

def tail_average(v,p,alpha):
    # Independent algorithm: consume fractional mass from the descending tail,
    # rather than evaluating the variational expression used by Go.
    mass=math.fsum(p);remain=1-alpha;total=0
    for x,w in sorted(zip(v,p),reverse=True):
        take=min(w/mass,remain);total+=take*x;remain-=take
        if remain<=0:break
    return total/(1-alpha)

def check_forecasts(path):
    it=iter(rows(path));header=next(it);cfg=header["config"]["forecast"];par=cfg["balance_parameters"]
    steps=records=forecasts=actions=quantiles=0
    for r in it:
        if r["type"]=="footer":break
        records+=1;payload=r["payload"];inp=payload["input"];out=payload["result"];fr=out["forecast"]
        if fr["status"]!="ready":assert not fr["ensemble"] and not fr["alternatives"];continue
        forecasts+=1;rr=inp["observation"]["readings"];value=lambda f:rr[f]["value"]
        initial=(value("raw_end"),[value("finished_end_a"),value("finished_end_b")],[value("backlog_after_a"),value("backlog_after_b")],inp["known_plan"]["base_plan"])
        est=fr["admission"]["estimate"];same(est["raw_estimate"],initial[0],"observed estimate")
        assert est["finished_estimate"]==initial[1] and est["backlog_estimate"]==initial[2] and est["registered_base_plan"]==initial[3]
        assert all(valid(rr[f]) and rr[f]["age_days"]==0 for f in fr["admission"]["required_fields"])
        paths={p["id"]:p for p in fr["ensemble"]["paths"]};diag=out["analysis"]["diagnosis"]
        post={h["id"]:h["posterior"] for h in diag["ranked"]};mass=sum(post[h] for h in diag["relevant_ids"])
        for group in fr["ensemble"]["groups"]:
            same(group["conditional_posterior"],post[group["id"]]/mass,"posterior mixture")
            ps=[p for p in paths.values() if p["hypothesis"]==group["id"]]
            assert len(ps)==cfg["paths_per_hypothesis"]
            for p in ps:same(p["probability"],group["conditional_posterior"]/len(ps),"group mass")
        for fa in fr["alternatives"]:
            actions+=1;u=fa["action"];zs=[];ws=[];allks=[]
            if u["id"]=="u6":
                rp=[min(1.75*par["base_demand"][i],max(.25*par["base_demand"][i],value(("demand_a","demand_b")[i])+.25*initial[2][i]-.1*initial[1][i])) for i in range(2)]
                for a,b in zip(rp,u["permanent_plan"]):same(a,b,"replan uses only current observations")
            for tr in fa["trajectories"]:
                p=paths[tr["scenario_id"]];state=(initial[0],list(initial[1]),list(initial[2]),list(initial[3]));ks=[];z=u["cost"]
                for h,w in enumerate(p["forecast_forcing"]):
                    state,k,loss=step(state,w,u,h,par);steps+=1;ks.append(k)
                    for j in range(4):same(k[j],tr["kpi"][h][j],"trajectory KPI")
                    same(loss,tr["period_losses"][h],"period loss")
                    z+=cfg["discount"]**h*loss
                same(z,tr["horizon_loss"],"one-off-cost discounted loss")
                fstate=tr["final_predicted_state_not_environment_truth"]
                for a,b in zip([state[0]]+state[1]+state[2]+state[3],[fstate["raw"]]+fstate["finished"]+fstate["backlog"]+fstate["plan"]):same(a,b,"predicted balance")
                zs.append(z);ws.append(p["probability"]);allks.append(ks)
            pred=fa["predicted_risk"];same(pred["expected"],sum(z*p for z,p in zip(zs,ws))/math.fsum(ws),"expected loss")
            same(pred["var"],quantile(zs,ws,cfg["alpha"]),"VaR")
            same(pred["cvar"],tail_average(zs,ws,cfg["alpha"]),"CVaR fractional tail")
            for h,b in enumerate(fa["bands"]):
                assert b["horizon_day"]==h+1
                for j in range(4):
                    v=[ks[h][j] for ks in allks]
                    same(b["mean"][j],sum(x*p for x,p in zip(v,ws)),"KPI mean")
                    same(b["lower"][j],quantile(v,ws,cfg["interval_quantiles"][0]),"lower quantile")
                    same(b["upper"][j],quantile(v,ws,cfg["interval_quantiles"][1]),"upper quantile");quantiles+=2
    return {"records_checked":records,"forecasts_checked":forecasts,"action_forecasts_checked":actions,"predicted_balance_steps_checked":steps,"weighted_quantiles_checked":quantiles}

def main():
    ap=argparse.ArgumentParser();ap.add_argument("--root",type=Path,default=Path(__file__).resolve().parents[1]);ap.add_argument("--out",type=Path)
    args=ap.parse_args();root=args.root
    result={"evidence":"independent arithmetic implementation, not external validation or effectiveness", "fit":check_fit(root/"results/fit_v04"),"forecasts":check_forecasts(root/"results/forecast_v04/forecast_trace.jsonl.gz")}
    result.update(passed=True,numeric_comparisons=COMPARISONS,maximum_absolute_error=MAX_ERROR)
    out=args.out or root/"checks/independent_reference_v04.json";out.parent.mkdir(parents=True,exist_ok=True);out.write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
    print(json.dumps({k:v for k,v in result.items() if k!="fit"},indent=2))
if __name__=="__main__":main()
