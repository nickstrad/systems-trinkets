# Daily Linux lesson prompt

A prompt for a scheduled chat thread that sends one short Linux/OS lesson per
day. It targets the mechanisms beneath the platforms studied in this
directory, excludes KVM/Firecracker per the local runtime boundary in
`AGENTS.md`, and requires every experiment to run inside a Docker container on
macOS. Paste the block below verbatim.

```
Send me a 5-minute Linux/OS lesson oriented around building and operating agent sandboxes, durable job runners, and isolated execution environments (the kind of platforms E2B, Daytona, Vercel Sandbox, Modal, Inngest, Temporal, and Meta's Muse are built on). I am learning this by writing small Go programs backed by PostgreSQL, Valkey, SeaweedFS (S3), and DuckDB, run locally on macOS with Docker Desktop. Linux-specific experiments must therefore be runnable inside a plain Docker container (for example `docker run --rm -it --privileged ubuntu` or an alpine/debian image), never on bare-metal Linux hardware features. Do not cover KVM, Firecracker, nested virtualization, or other VM runtimes; I have deliberately dropped those. The lessons are about the Linux mechanisms that sit underneath every such platform regardless of whether the outer isolation boundary is a container, gVisor, nspawn cell, or VM.

Each lesson should teach exactly one practical Linux or operating-systems concept, using one or two common CLI tools, a tiny terminal experiment I can run in a container in about 5 minutes, and a concise explanation of why this concept matters when you are the one building a sandbox platform, a supervisor, or a durable job runner. Where natural, end with one sentence on how the concept would show up in a Go program (a syscall wrapper, an os/exec option, a signal handler, a file descriptor to watch), but keep the lesson itself CLI-first.

Build progressively across topics such as:
- Process lifecycle: fork/exec, process trees, zombies, orphan reparenting, PID 1 and init responsibilities, minimal supervisors (tini, dumb-init, systemd-nspawn ideas), what a "sandbox exec" actually does
- /proc and /sys as observability: /proc/<pid>/status, fd, maps, cgroup, ns, mountinfo, limits
- File descriptors: inheritance across exec, close-on-exec, pipes, pseudo-terminals (pty) for interactive shells, epoll and io_uring basics
- Signals: SIGTERM vs SIGKILL, graceful shutdown, signal forwarding through a supervisor, timeouts and idle-kill
- Namespaces: pid, mount, net, user, uts, ipc; unshare and nsenter; user namespaces and uid/gid remapping; rootless containers
- cgroups v2: CPU and memory limits, memory.max, OOM behavior, pids.max, reading usage stats, why per-job accounting matters
- Capabilities, seccomp syscall filters, no_new_privs, and what "filtered syscalls/capabilities" means for a workspace cell
- Filesystems and mounts: bind mounts, read-only mounts, tmpfs, overlayfs and image layers, copy-on-write, volumes, loop devices, block device basics without a hypervisor
- Snapshot and restore ideas: page cache, mmap and lazy loading, sparse files, reflinks, why "restore from a template" is fast, CRIU-style checkpoint concepts at a conceptual level
- Networking inside isolation: network namespaces, veth pairs, bridges, routing tables, iptables/nftables, DNS resolution inside a sandbox, egress allow-lists, forward proxies, the basics of eBPF-based filtering
- Local IPC and identity: Unix domain sockets, SO_PEERCRED, socket file permissions, why a peer credential beats a JSON "identity" field
- Resource limits and pressure: ulimit/rlimits, nofile, nproc, disk quotas, PSI (pressure stall information), what happens at the limit
- Tracing and debugging: strace, ltrace, lsof, ss, perf, bpftrace basics, reading a stuck process, finding what holds a file or port
- Measurement: timing cold vs warm starts, page-cache effects on repeated runs, measuring with /usr/bin/time, hyperfine, and reading numbers honestly
- Durability primitives the platforms rely on: fsync and write ordering, O_DIRECT, atomic rename, file locks, leases and heartbeats, what a "durable timer" needs from the OS

Prefer common Linux CLI tools already in ubuntu/debian/alpine images (or installable with one apt/apk line), keep the whole lesson doable in about 5 minutes, and always say up front if the experiment needs --privileged or a specific capability so I can start the container correctly.
```
