#!/usr/bin/env python3
"""Golden-fixture generator for spec 002 (run against the LEGACY Python builder).
Produces ~/Games/nms-modding/golden/: per-target merged MXML + report lines + per-script dump JSON.
Game-derived output; never commit it. Runs the legacy builder's merge stage (no compile) and
and writes per-target merged MXML + report lines, plus per-script dumped JSON."""
import sys, os, json, shutil, subprocess, importlib.util
W=os.path.expanduser("~/Games/nms-modding")
G=f"{W}/golden"
spec=importlib.util.spec_from_file_location("nb", f"{W}/builder/nms_build_mods.py")
nb=importlib.util.module_from_spec(spec); spec.loader.exec_module(nb)
shutil.rmtree(G, ignore_errors=True); os.makedirs(f"{G}/merged"); os.makedirs(f"{G}/dumped")
shutil.copy(f"{W}/mods.conf", f"{G}/mods.conf"); shutil.copy(f"{W}/cache/manifest.json", f"{G}/manifest.json")
mods=nb.load_config()
targets={}; rep=nb.Report(); order=[]
for name,en in mods:
    if not en: continue
    lf=f"{nb.LUA_SRC}/{name}.lua"
    j=subprocess.run(["lua",nb.DUMPER,lf],capture_output=True,text=True,check=True).stdout
    open(f"{G}/dumped/{name}.json","w").write(j)
    C=json.loads(j)
    for mod in C.get("MODIFICATIONS",[]):
        for mct in mod.get("MBIN_CHANGE_TABLE",[]):
            srcv=mct.get("MBIN_FILE_SOURCE")
            if not srcv: continue
            srclist=[srcv] if isinstance(srcv,str) else [x for x in srcv if isinstance(x,str)]
            for src in srclist:
                for blk in mct.get("EXML_CHANGE_TABLE",[]):
                    k=nb.norm(src)
                    if k not in targets: order.append(k)
                    targets.setdefault(k,[]).append((name,src,blk))
index=[]
for tgt in order:
    items=targets[tgt]
    mxml,internal=nb.mxml_for(items[0][1])
    if not mxml or not os.path.exists(mxml):
        index.append({"target":tgt,"internal":None,"blocks":len(items),"merged":None}); continue
    lines=open(mxml,encoding="utf-8").read().split("\n")
    for name,src,blk in items:
        try: nb.apply_block(lines,blk,rep,name,src)
        except Exception as ex: rep.warn(name,f"exception on {os.path.basename(src)}: {ex}",src)
    out=f"{G}/merged/{internal.upper()}"
    os.makedirs(os.path.dirname(out),exist_ok=True)
    open(out,"w",encoding="utf-8").write("\n".join(lines))
    index.append({"target":tgt,"internal":internal.upper(),"blocks":len(items),"merged":os.path.relpath(out,G),"source_mxml":os.path.relpath(mxml,W)})
json.dump({"targets":index,"applied":rep.applied,"skipped":rep.skipped,
           "per_mod":{k:{"applied":v["applied"],"skipped":v["skipped"],"notfound":v["notfound"]} for k,v in rep.mod.items()}},
          open(f"{G}/index.json","w"),indent=1)
open(f"{G}/report_lines.txt","w").write("\n".join(rep.lines)+"\n")
shutil.copy(f"{W}/BUILD_REPORT.md", f"{G}/BUILD_REPORT.md")
print(f"targets={len(order)} applied={rep.applied} skipped={rep.skipped}")
