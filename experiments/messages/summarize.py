#!/usr/bin/env python3
import argparse
import csv
import json
from pathlib import Path
import re
import statistics


def expected_groups(config,inventory):
    patterns=config["pattern"].split("/")
    root=patterns[0].strip("^$")
    if root not in {"BenchmarkBytes","BenchmarkParallelBytes","BenchmarkJavaScriptBytes"}:
        raise ValueError("unknown suite")
    expected=set()
    for row in inventory:
        engine,kind,size,n=row["engine"],row["kind"],row["target_bytes"],row["fanout"]
        if root=="BenchmarkBytes" and engine=="javascript":continue
        if root=="BenchmarkJavaScriptBytes" and engine!="javascript":continue
        if root=="BenchmarkParallelBytes" and (engine=="javascript" or kind not in {"scalar","object","array","bindings"} or size!=1024):continue
        for strategy in ["direct","prepared","batch"]:
            parts=[root,engine,kind,str(size),"N"+str(n),strategy]
            if len(patterns)>len(parts) or not all(re.search(pattern,parts[i]) for i,pattern in enumerate(patterns)):continue
            for cpu in config["cpu"].split(","):expected.add(("/".join(parts),int(cpu)))
    return expected


def summarize(directory):
    metadata=json.loads((directory/"metadata.json").read_text())
    if not metadata.get("completed") or not metadata.get("source_sha256") or metadata.get("final_source_sha256")!=metadata["source_sha256"]:
        raise ValueError("incomplete or changed source")
    if set(metadata["profiles"])!=set(metadata["requested_profiles"]):raise ValueError("missing requested profiles")
    inventory=json.loads((directory/"inventory.json").read_text())
    rows=[]
    for profile,config in metadata["profiles"].items():
        text=(directory/(profile+".txt")).read_text()
        if text.splitlines().count("PASS")!=metadata["rounds"]:
            raise ValueError(f"incomplete profile {profile}")
        samples={}
        for line in text.splitlines():
            if not line.startswith("Benchmark"):continue
            fields=line.split();name=fields[0]
            match=re.search(r"-(\d+)$",name)
            workers=int(match.group(1)) if match else 1
            name=re.sub(r"-\d+$","",name)
            metrics=[float(fields[fields.index(unit)-1]) for unit in ["ns/op","B/op","allocs/op"]]
            samples.setdefault((name,workers),[]).append(metrics)
        if not samples or any(len(v)!=metadata["rounds"] for v in samples.values()):
            raise ValueError(f"bad sample counts {profile}")
        if set(samples)!=expected_groups(config,inventory):
            raise ValueError(f"missing/unexpected workload, strategy or CPU group: {profile}")
        runs=[c for c in metadata["commands"] if c["profile"]==profile]
        if sorted(c["round"] for c in runs)!=list(range(1,metadata["rounds"]+1)):
            raise ValueError(f"incomplete profile command rounds: {profile}")
        for (name,workers),values in samples.items():
            parts=name.split("/")
            ns=[v[0] for v in values]
            rows.append({"profile":profile,"benchmark":name,"workers":workers,"engine":parts[1],"kind":parts[2],"size":int(parts[3]),"fanout":int(parts[4][1:]),"strategy":parts[5],
                         "ns_median":statistics.median(ns),"ns_min":min(ns),"ns_max":max(ns),"bytes_median":statistics.median(v[1] for v in values),"allocs_median":statistics.median(v[2] for v in values)})
    if len(metadata["commands"])!=metadata["rounds"]*len(metadata["profiles"]) or any(c["exit_code"] for c in metadata["commands"]):
        raise ValueError("incomplete command manifest")
    with (directory/"summary.csv").open("w",newline="") as f:
        writer=csv.DictWriter(f,fieldnames=list(rows[0]),lineterminator="\n");writer.writeheader();writer.writerows(rows)
    (directory/"summary.json").write_text(json.dumps(rows,indent=2)+"\n")
    selected=[r for r in rows if r["profile"]=="native127" and r["engine"]=="jsonata" and r["size"]==1024 and r["kind"] in ["scalar","object","array","bindings"]]
    lines=["# Native message results","",f'{metadata["rounds"]}-sample warm medians. All paths include fresh input handling and encoded outputs. Values are per input message, including its complete fan-out. ns/op for parallel profiles is aggregate wall time divided by completed messages, not individual latency.',"","| JSONata, 1 KiB target | Fan-out | Strategy | us/message | B/message | Allocations |","|---|---:|---|---:|---:|---:|"]
    for r in selected:lines.append(f'| {r["kind"]} | {r["fanout"]} | {r["strategy"]} | {r["ns_median"]/1000:.2f} | {r["bytes_median"]:.0f} | {r["allocs_median"]:.0f} |')
    lines += ["","Payload sizes vary; inventory.json records actual sizes and exact expression/corpus identities. Parallel results, controls, JavaScript baseline and all ranges are in summary.csv/summary.json. No NATS transport or tail latency is measured here."]
    (directory/"summary.md").write_text("\n".join(lines)+"\n")


if __name__=="__main__":
    parser=argparse.ArgumentParser();parser.add_argument("directory",type=Path);summarize(parser.parse_args().directory)
