import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
const root=path.dirname(fileURLToPath(import.meta.url));
const manifest=JSON.parse(fs.readFileSync(path.join(root,'MANIFEST_SHA256.json'),'utf8'));
const bad=[];
for(const [name,expected] of Object.entries(manifest)){
 try{
  const target=path.resolve(root,name);
  if(!target.startsWith(root+path.sep)||name.split('/').includes('..')) throw new Error('unsafe path');
  const actual=crypto.createHash('sha256').update(fs.readFileSync(target)).digest('hex');
  if(actual!==expected) bad.push(name);
 }catch{bad.push(name);}
}
console.log(`Checked ${Object.keys(manifest).length} files; mismatches: ${bad.length}`);
for(const name of bad) console.error(name);
process.exitCode=bad.length?1:0;
