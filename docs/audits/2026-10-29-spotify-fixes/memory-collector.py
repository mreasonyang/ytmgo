import argparse,ctypes,datetime,json,pathlib,re,subprocess,time
p=argparse.ArgumentParser();p.add_argument('--phase',required=True);p.add_argument('--count',type=int,default=7);p.add_argument('--interval',type=float,default=5);a=p.parse_args()
# Only executable identity and resource counters are collected. No argv,
# environment, authentication files or application log content are accessed.
base=pathlib.Path('/Users/easonyang/Documents/projects/ytmgo/build/darwin-arm64/ytmgo.app/Contents')
sdk=pathlib.Path('/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/usr/include/sys/resource.h').read_text()
body=sdk.split('struct rusage_info_v4 {',1)[1].split('};',1)[0]
fields=re.findall(r'uint64_t\s+(\w+);',body)
class Usage(ctypes.Structure):_fields_=[('ri_uuid',ctypes.c_uint8*16)]+[(n,ctypes.c_uint64) for n in fields]
lib=ctypes.CDLL('/usr/lib/libproc.dylib');lib.proc_pid_rusage.argtypes=[ctypes.c_int,ctypes.c_int,ctypes.c_void_p];lib.proc_pid_rusage.restype=ctypes.c_int
lib.proc_pidpath.argtypes=[ctypes.c_int,ctypes.c_void_p,ctypes.c_uint32];lib.proc_pidpath.restype=ctypes.c_int
out=pathlib.Path('/Users/easonyang/Documents/projects/ytmgo/.mygo/spotify-memory/samples.jsonl')
for i in range(a.count):
 rows=json.loads(subprocess.check_output(['/Users/easonyang/.codex/bin/safe-processes','--name','ytmgo'],text=True));processes=[]
 for row in rows:
  exe=pathlib.Path(row['executable'])
  if base not in exe.parents:continue
  buf=ctypes.create_string_buffer(4096)
  if lib.proc_pidpath(row['pid'],buf,len(buf))<=0 or buf.value.decode()!=str(exe):continue
  u=Usage()
  if lib.proc_pid_rusage(row['pid'],4,ctypes.byref(u))!=0:continue
  processes.append({'pid':row['pid'],'ppid':row['ppid'],'executable':str(exe.relative_to(base)),'resident_bytes':u.ri_resident_size,'physical_footprint_bytes':u.ri_phys_footprint,'lifetime_peak_footprint_bytes':u.ri_lifetime_max_phys_footprint})
 sample={'time_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'phase':a.phase,'index':i,'processes':processes,'total_footprint_bytes':sum(r['physical_footprint_bytes'] for r in processes),'total_resident_bytes':sum(r['resident_bytes'] for r in processes)}
 with out.open('a') as f:f.write(json.dumps(sample)+'\n')
 print(json.dumps({'phase':a.phase,'sample':i,'processes':[{'pid':r['pid'],'executable':r['executable'],'footprint_mib':round(r['physical_footprint_bytes']/1048576,2),'rss_mib':round(r['resident_bytes']/1048576,2),'lifetime_peak_mib':round(r['lifetime_peak_footprint_bytes']/1048576,2)} for r in processes],'total_footprint_mib':round(sample['total_footprint_bytes']/1048576,2),'total_rss_mib':round(sample['total_resident_bytes']/1048576,2)}),flush=True)
 if i+1<a.count:time.sleep(a.interval)
