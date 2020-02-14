package lib
// #include <sys/types.h>
// #include <sys/stat.h>
// #include <fcntl.h>
// #include <unistd.h> // for close
// #include <sys/eventfd.h>
// #include <errno.h>
// #include <string.h>
// #include <stdio.h>
// #include <stdlib.h>
// 
// static inline void die(const char *msg)
// {
// 	fprintf(stderr, "error: %s: %s(%d)\n", msg, strerror(errno), errno);
// 	exit(EXIT_FAILURE);
// }
//
// #define BUFSIZE 256
// 
// static void listen_for_oom(char * const event_control_path, char * const oom_control_path)
// {
// 	char buf[BUFSIZE];
// 	int efd, cfd, ofd, rb, wb;
// 	uint64_t u;
// 
// 	if ((efd = eventfd(0, 0)) == -1)
// 		die("eventfd");
// 
// 	if ((cfd = open(event_control_path, O_WRONLY)) == -1)
// 		die("cgroup.event_control");
// 
// 	if ((ofd = open(oom_control_path, O_RDONLY)) == -1)
// 		die("memory.oom_control");
// 
// 	if ((wb = snprintf(buf, BUFSIZE, "%d %d", efd, ofd)) >= BUFSIZE)
// 		die("buffer too small");
// 
// 	if (write(cfd, buf, wb) == -1)
// 		die("write cgroup.event_control");
// 
// 	if (close(cfd) == -1)
// 		die("close cgroup.event_control");
// 
// 	for (;;) {
// 		if (read(efd, &u, sizeof(uint64_t)) != sizeof(uint64_t))
// 			die("read eventfd");
// 
// 		printf("mem_cgroup oom event received\n");
// 	}
// 
// 	return;
// }
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"github.com/pkg/errors"
)

func (c *conmonmon) registerConmon(info *conmonInfo, cgroupv2 bool) error {
	fmt.Fprintf(os.Stderr, "finding cgroup files for %d\n", info.conmonPID)
	cgroupMemoryPath, err := processCgroupSubsystemPath(info.conmonPID, cgroupv2, "memory")
	if err != nil {
		return errors.Wrapf(err, "failed to get event_control file for pid %d", info.conmonPID)
	}

	fmt.Fprintf(os.Stderr, "found cgroup subsystem path %s for %d\n", cgroupMemoryPath, info.conmonPID)

	oomControl := C.CString(filepath.Join(cgroupMemoryPath, "memory.oom_control"))
	eventControl := C.CString(filepath.Join(cgroupMemoryPath, "cgroup.event_control"))

	fmt.Fprintf(os.Stderr, "got %s %s\n", oomControl, eventControl)

	go func() {
		C.listen_for_oom(eventControl, oomControl)
	}()

	return nil
}
