import unittest

from trace_summary import summarize


class TraceRanges(unittest.TestCase):
    def test_only_gc_world_stops_are_counted(self):
        lines = [
            'M=1 P=0 G=1 RangeBegin Time=1000 Name="stop-the-world (start trace)" Scope=Goroutine(1)',
            'M=1 P=0 G=1 RangeBegin Time=2000 Name="stop-the-world (GC sweep termination)" Scope=Goroutine(1)',
            'M=1 P=0 G=1 RangeEnd Time=12000 Name="stop-the-world (GC sweep termination)" Scope=Goroutine(1) Attributes=[]',
            'M=2 P=0 G=8 RangeBegin Time=15000 Name="stop-the-world (GC mark termination)" Scope=Goroutine(8)',
            'M=2 P=0 G=8 RangeEnd Time=35000 Name="stop-the-world (GC mark termination)" Scope=Goroutine(8) Attributes=[]',
        ]
        self.assertEqual(summarize(lines), {'gc_stw_ranges': 2, 'total_us': 30, 'p50_us': 10, 'p95_us': 20, 'p99_us': 20, 'max_us': 20})
        for bad in [lines[2:3], lines[1:2], lines[1:2]*2]:
            with self.assertRaises(ValueError): summarize(bad)


if __name__ == '__main__':
    unittest.main()
