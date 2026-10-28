#include <libproc.h>
#include <sys/resource.h>
#include <sys/proc_info.h>
#include <mach/mach_time.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <errno.h>
#include <string.h>
int main(int argc, char **argv) {
  mach_timebase_info_data_t tb; mach_timebase_info(&tb);
  for (int i=1;i<argc;i++) {
    pid_t pid=(pid_t)strtol(argv[i],NULL,10);
    struct rusage_info_v4 r={0};
    if (proc_pid_rusage(pid,RUSAGE_INFO_V4,(rusage_info_t *)&r)!=0) {
      printf("{\"pid\":%d,\"error_errno\":%d}\n",pid,errno);continue;
    }
    struct proc_taskinfo t={0};
    int valid=proc_pidinfo(pid,PROC_PIDTASKINFO,0,&t,sizeof(t))==sizeof(t);
    double uptime=((double)mach_absolute_time()-r.ri_proc_start_abstime)*tb.numer/tb.denom/1e9;
    printf("{\"pid\":%d,\"start_ticks\":%llu,\"age_seconds\":%.3f,\"footprint\":%llu,\"resident\":%llu,\"lifetime_peak\":%llu,\"cpu_ns\":%llu,\"threads\":%d,\"taskinfo_available\":%s}\n",pid,r.ri_proc_start_abstime,uptime,r.ri_phys_footprint,r.ri_resident_size,r.ri_lifetime_max_phys_footprint,r.ri_user_time+r.ri_system_time,valid?t.pti_threadnum:-1,valid?"true":"false");
  }
  return 0;
}
