import csv
import json
from pathlib import Path
import tempfile
import unittest
from summarize import summarize

class SummaryTests(unittest.TestCase):
    def fixture(self, directory):
        (directory/'metadata.json').write_text(json.dumps({'rounds':2,'requested_profiles':['parallel127'],'completed':True,'source_sha256':'same','final_source_sha256':'same','profiles':{'parallel127':{'pattern':'^BenchmarkParallelBytes$/jsonata/scalar/1024/N4$','cpu':'8'}},'commands':[{'exit_code':0,'profile':'parallel127','round':1},{'exit_code':0,'profile':'parallel127','round':2}]}))
        (directory/'inventory.json').write_text(json.dumps([{'engine':'jsonata','kind':'scalar','target_bytes':1024,'fanout':4}]))
        chunks=[]
        for ns,bytes_,allocs in [(100,40,2),(300,60,4)]:
            chunks.extend(f'BenchmarkParallelBytes/jsonata/scalar/1024/N4/{strategy}-8 100 {ns} ns/op {bytes_} B/op {allocs} allocs/op\n' for strategy in ['direct','prepared','batch'])
            chunks.append('PASS\n')
        (directory/'parallel127.txt').write_text(''.join(chunks))
    def test_rounds_units_and_concurrency(self):
        with tempfile.TemporaryDirectory() as tmp:
            d=Path(tmp);self.fixture(d);summarize(d)
            row=json.loads((d/'summary.json').read_text())[0]
            self.assertEqual((row['workers'],row['fanout'],row['ns_median'],row['bytes_median'],row['allocs_median']),(8,4,200,50,3))
            self.assertNotIn(b'\r',(d/'summary.csv').read_bytes())
    def test_incomplete_and_failed_runs_are_rejected(self):
        for failure in ['missing_pass','missing_sample','command_error','source_changed','missing_strategy','missing_cpu']:
            with self.subTest(failure=failure),tempfile.TemporaryDirectory() as tmp:
                d=Path(tmp);self.fixture(d)
                if failure=='missing_pass':(d/'parallel127.txt').write_text((d/'parallel127.txt').read_text().replace('PASS','',1))
                if failure=='missing_sample':(d/'parallel127.txt').write_text('PASS\nPASS\n')
                if failure=='command_error':
                    meta=json.loads((d/'metadata.json').read_text());meta['commands'][1]['exit_code']=1;(d/'metadata.json').write_text(json.dumps(meta))
                if failure=='source_changed':
                    meta=json.loads((d/'metadata.json').read_text());meta['completed']=False;meta['final_source_sha256']='changed';(d/'metadata.json').write_text(json.dumps(meta))
                if failure=='missing_strategy':
                    (d/'parallel127.txt').write_text('\n'.join(line for line in (d/'parallel127.txt').read_text().splitlines() if '/direct-' not in line)+'\n')
                if failure=='missing_cpu':
                    meta=json.loads((d/'metadata.json').read_text());meta['profiles']['parallel127']['cpu']='1,8';(d/'metadata.json').write_text(json.dumps(meta))
                with self.assertRaises(ValueError):summarize(d)
if __name__=='__main__':unittest.main()
