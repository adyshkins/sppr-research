import unittest
from analyze_factorial_v06 import quantile,empirical_cvar,bootstrap
class StatisticsTests(unittest.TestCase):
 def test_quantile_linear(self):self.assertEqual(quantile([0,10,20],.25),5)
 def test_constant_tail(self):self.assertAlmostEqual(empirical_cvar([7]*60),7)
 def test_fractional_mass(self):self.assertAlmostEqual(empirical_cvar([1,2,3,10],.625),(10+1.5)/1.5)
 def test_entire_sample_limit(self):self.assertAlmostEqual(empirical_cvar([1,2,3,10],0),4)
 def test_paired_common_offset_cancels(self):
  a=[2,3,4];b=[3,5,7];v=[y-x for x,y in zip(a,b)];self.assertEqual(bootstrap(v,[[0,1,2]]*10),[2,2])
 def test_identical_pairs_interval_zero(self):self.assertEqual(bootstrap([0]*12,[list(range(12))]*100),[0,0])
if __name__=='__main__':unittest.main()
