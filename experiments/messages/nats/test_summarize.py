import copy
import json
from pathlib import Path
import tempfile
import unittest
from summarize import summarize,expected_keys

class Reports(unittest.TestCase):
    def fixture(self,d):
        meta={'completed':True,'container_removed':True,'cases_per_round':6,'rounds':1,'duration':'1s'}
        (d/'metadata.json').write_text(json.dumps(meta));rows=[]
        for key in sorted(expected_keys(meta)):
            round_,engine,kind,n,w,rate,count,queue,size,strategy=key
            c=dict(zip(['engine','kind','fanout','workers','target_rate','count','queue_capacity','target_bytes','strategy'],[engine,kind,n,w,rate,count,queue,size,strategy]))
            r={'config':c,'valid':True,'sent':count,'received':count,'processed':count,'completed':count,'outputs':count*n,'queue_rejected':0,'transform_errors':0,'lossless':True,
               'actual_offered_per_second':500,'completed_per_second':490,'max_pacing_lateness_ns':100,'allocated_bytes_during_run':1000}
            for error in ['publish_errors','invalid_outputs','async_errors','input_pending_dropped','output_pending_dropped','unaccounted_inputs','incomplete_fanout']:r[error]=0
            for label in ['processing','queue_wait','end_to_end']:r[label]={'p50_ns':1000,'p95_ns':2000,'p99_ns':3000}
            rows.append({'round':round_,'report':r})
        self.write(d,rows);return rows
    def write(self,d,rows):(d/'runs.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in rows))
    def test_focused_long_matrix(self):
        meta={'workload_profile':'object-one-worker','cases_per_round':6,'rounds':1,'duration':'10s'}
        keys=expected_keys(meta)
        self.assertEqual(len(keys),6)
        self.assertTrue(all(k[1:3]==('jsonata','object') and k[4:9]==(1,5000,50000,64,1024) for k in keys))
        for duration in ['11s','0s','nan','1ns']:
            with self.subTest(duration=duration),self.assertRaises(ValueError):
                expected_keys({**meta,'duration':duration})

    def test_complete_and_lossy_accounting(self):
        with tempfile.TemporaryDirectory() as tmp:
            d=Path(tmp);rows=self.fixture(d)
            r=rows[0]['report'];r.update(processed=127,completed=127,outputs=127*r['config']['fanout'],queue_rejected=1,lossless=False)
            self.write(d,rows);summarize(d)
            out=json.loads((d/'summary.json').read_text());self.assertEqual(len(out),6);self.assertEqual(min(x['lossless_rounds'] for x in out),0)
    def test_missing_bad_and_incomplete_are_rejected(self):
        for change in ['missing','duplicate','invalid','cleanup']:
            with self.subTest(change=change),tempfile.TemporaryDirectory() as tmp:
                d=Path(tmp);rows=self.fixture(d)
                if change=='missing':rows.pop()
                if change=='duplicate':rows.append(copy.deepcopy(rows[0]))
                if change=='invalid':rows[0]['report']['completed']-=1
                if change=='cleanup':
                    meta=json.loads((d/'metadata.json').read_text());meta['container_removed']=False;(d/'metadata.json').write_text(json.dumps(meta))
                self.write(d,rows)
                with self.assertRaises(ValueError):summarize(d)
if __name__=='__main__':unittest.main()
