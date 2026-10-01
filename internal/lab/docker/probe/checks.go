//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func cmdID(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("id"), args, 0, 0); err != nil {
		return err
	}
	groups, err := os.Getgroups()
	if err != nil {
		r.deny("identity", err)
		return nil
	}
	strs := make([]string, len(groups))
	for i, g := range groups {
		strs[i] = strconv.Itoa(g)
	}
	r.ok("identity", "uid=%d gid=%d euid=%d egid=%d groups=%s",
		os.Getuid(), os.Getgid(), os.Geteuid(), os.Getegid(), strings.Join(strs, ","))
	return nil
}

// statusFields maps a /proc/self/status field to its probe line name.
var statusFields = []struct{ field, name string }{
	{"CapEff", "cap-eff"},
	{"NoNewPrivs", "no-new-privs"},
	{"Seccomp", "seccomp"},
}

func cmdStatus(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("status"), args, 0, 0); err != nil {
		return err
	}
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		for _, f := range statusFields {
			r.deny(f.name, err)
		}
		return nil
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			values[k] = strings.TrimSpace(v)
		}
	}
	for _, f := range statusFields {
		if v, ok := values[f.field]; ok {
			r.ok(f.name, "%s", v)
		} else {
			r.deny(f.name, fmt.Errorf("no %s field in /proc/self/status", f.field))
		}
	}
	return nil
}

func cmdPIDs(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("pids"), args, 0, 0); err != nil {
		return err
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		r.deny("pids", err)
		return nil
	}
	var pids []int
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil {
			pids = append(pids, n)
		}
	}
	sort.Ints(pids)
	strs := make([]string, len(pids))
	for i, p := range pids {
		strs[i] = strconv.Itoa(p)
	}
	r.ok("pids", "%s", strings.Join(strs, " "))
	return nil
}

func cmdInterfaces(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("interfaces"), args, 0, 0); err != nil {
		return err
	}
	ifs, err := net.Interfaces()
	if err != nil {
		r.deny("interfaces", err)
		return nil
	}
	names := make([]string, len(ifs))
	for i, ifc := range ifs {
		names[i] = ifc.Name
	}
	r.ok("interfaces", "%s", strings.Join(names, " "))
	return nil
}

// checkWrite creates a file in dir, writes to it and removes it. The line is
// named name so a battery can report several places.
func checkWrite(r *reporter, name, dir string) {
	f, err := os.CreateTemp(dir, ".probe-write-")
	if err == nil {
		_, err = f.WriteString("probe\n")
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		_ = os.Remove(f.Name())
	}
	r.result(name, dir, err)
}

func cmdWrite(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("write"), args, 1, 1)
	if err != nil {
		return err
	}
	checkWrite(r, "write", filepath.Clean(rest[0]))
	return nil
}

func checkSetuid(r *reporter, uid int) {
	r.result("setuid", "uid="+strconv.Itoa(uid), syscall.Setuid(uid))
}

func cmdSetuid(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("setuid"), args, 0, 1)
	if err != nil {
		return err
	}
	uid := 0
	if len(rest) == 1 {
		if uid, err = strconv.Atoi(rest[0]); err != nil || uid < 0 {
			return usagef("uid %q is not a non-negative integer", rest[0])
		}
	}
	checkSetuid(r, uid)
	return nil
}

// parseOwner parses "uid:gid".
func parseOwner(s string) (uid, gid int, err error) {
	u, g, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, usagef("owner %q is not uid:gid", s)
	}
	if uid, err = strconv.Atoi(u); err != nil || uid < 0 {
		return 0, 0, usagef("uid %q is not a non-negative integer", u)
	}
	if gid, err = strconv.Atoi(g); err != nil || gid < 0 {
		return 0, 0, usagef("gid %q is not a non-negative integer", g)
	}
	return uid, gid, nil
}

// parseMode parses an octal mode with the setuid, setgid and sticky bits
// (07777), as chmod(2) takes it.
func parseMode(s string) (uint32, error) {
	m, err := strconv.ParseUint(s, 8, 32)
	if err != nil || m > 0o7777 {
		return 0, usagef("mode %q is not octal in 0..7777", s)
	}
	return uint32(m), nil
}

func cmdChown(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("chown"), args, 2, 2)
	if err != nil {
		return err
	}
	uid, gid, err := parseOwner(rest[1])
	if err != nil {
		return err
	}
	r.result("chown", rest[0]+" "+rest[1], os.Chown(rest[0], uid, gid))
	return nil
}

func cmdChmod(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("chmod"), args, 2, 2)
	if err != nil {
		return err
	}
	mode, err := parseMode(rest[1])
	if err != nil {
		return err
	}
	// syscall.Chmod takes the raw bits; os.Chmod would need os.ModeSetgid.
	r.result("chmod", rest[0]+" "+rest[1], pathErr("chmod", rest[0], syscall.Chmod(rest[0], mode)))
	return nil
}

func cmdUnlink(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("unlink"), args, 1, 1)
	if err != nil {
		return err
	}
	r.result("unlink", rest[0], pathErr("unlink", rest[0], syscall.Unlink(rest[0])))
	return nil
}

func cmdStat(args []string, r *reporter) error {
	rest, err := parseFlags(newFlagSet("stat"), args, 1, 1)
	if err != nil {
		return err
	}
	var st syscall.Stat_t
	if err := syscall.Lstat(rest[0], &st); err != nil {
		r.deny("stat", pathErr("lstat", rest[0], err))
		return nil
	}
	r.ok("stat", "%s mode=%s perm=%04o uid=%d gid=%d", rest[0], fileMode(st.Mode), st.Mode&0o7777, st.Uid, st.Gid)
	return nil
}

// fileMode renders the type and permission bits like ls does (srwxrwx---,
// drwxr-s--- for a setgid directory).
func fileMode(mode uint32) string {
	var t byte
	switch mode & syscall.S_IFMT {
	case syscall.S_IFSOCK:
		t = 's'
	case syscall.S_IFDIR:
		t = 'd'
	case syscall.S_IFLNK:
		t = 'l'
	case syscall.S_IFIFO:
		t = 'p'
	case syscall.S_IFCHR:
		t = 'c'
	case syscall.S_IFBLK:
		t = 'b'
	default:
		t = '-'
	}
	b := []byte("rwxrwxrwx")
	for i := range b {
		if mode&(1<<(8-i)) == 0 {
			b[i] = '-'
		}
	}
	// setuid, setgid and sticky show in the execute slot: lower case when the
	// execute bit is also set, upper case when it is not.
	for _, f := range []struct {
		bit  uint32
		slot int
		on   byte
	}{{syscall.S_ISUID, 2, 's'}, {syscall.S_ISGID, 5, 's'}, {syscall.S_ISVTX, 8, 't'}} {
		if mode&f.bit != 0 {
			b[f.slot] = f.on
			if mode&(1<<(8-f.slot)) == 0 {
				b[f.slot] -= 'a' - 'A'
			}
		}
	}
	return string(t) + string(b)
}

// pathErr wraps a syscall error with the operation and path, as the os
// package does, and keeps nil as nil.
func pathErr(op, path string, err error) error {
	if err == nil {
		return nil
	}
	return &os.PathError{Op: op, Path: path, Err: err}
}

// checkMount mounts a tmpfs on /dev/shm (which Docker always provides) and
// unmounts it again. The target exists, so a refusal is a permission answer
// rather than a missing directory.
func checkMount(r *reporter) {
	const target = "/dev/shm"
	err := syscall.Mount("tmpfs", target, "tmpfs", 0, "")
	if err == nil {
		_ = syscall.Unmount(target, 0)
	}
	r.result("mount", target, pathErr("mount", target, err))
}

func cmdMount(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("mount"), args, 0, 0); err != nil {
		return err
	}
	checkMount(r)
	return nil
}

// checkUnshareUser starts a child in a new user namespace. The unshare(2)
// call itself cannot be probed from Go: the runtime is multi-threaded, and
// unshare(CLONE_NEWUSER) then fails with EINVAL before it checks permission,
// so an allowed and a denied container would look alike. clone(2) with the
// same flag goes through the same permission and seccomp checks.
func checkUnshareUser(r *reporter) {
	cmd := exec.Command("/proc/self/exe", "noop")
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWUSER}
	r.result("unshare-user", "CLONE_NEWUSER", cmd.Run())
}

func cmdUnshareUser(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("unshare-user"), args, 0, 0); err != nil {
		return err
	}
	checkUnshareUser(r)
	return nil
}

// keyctl commands and key specials from <linux/keyctl.h>.
const (
	keyctlGetKeyringID = 0
	keySpecThread      = -1
)

// checkKeyctl asks for the thread keyring, creating it. The default Docker
// seccomp profile rejects the keyctl syscall outright.
func checkKeyctl(r *reporter) {
	spec := keySpecThread
	_, _, errno := syscall.Syscall(syscall.SYS_KEYCTL, keyctlGetKeyringID, uintptr(spec), 1)
	if errno != 0 {
		r.deny("keyctl", errno)
		return
	}
	r.ok("keyctl", "GET_KEYRING_ID")
}

func cmdKeyctl(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("keyctl"), args, 0, 0); err != nil {
		return err
	}
	checkKeyctl(r)
	return nil
}

func checkCgroup(r *reporter) {
	for _, f := range []string{"memory.max", "pids.max", "cpu.max"} {
		data, err := os.ReadFile("/sys/fs/cgroup/" + f)
		r.result(f, strings.TrimSpace(string(data)), err)
	}
}

func cmdCgroup(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("cgroup"), args, 0, 0); err != nil {
		return err
	}
	checkCgroup(r)
	return nil
}

// cmdBattery runs the fixed survey the isolation table needs, in one exec.
// Checks that take arguments (write to a given path, chown, dial) are separate
// commands.
func cmdBattery(args []string, r *reporter) error {
	if _, err := parseFlags(newFlagSet("battery"), args, 0, 0); err != nil {
		return err
	}
	for _, c := range []func([]string, *reporter) error{cmdID, cmdStatus, cmdPIDs, cmdInterfaces} {
		if err := c(nil, r); err != nil {
			return err
		}
	}
	checkWrite(r, "write-root", "/")
	checkMount(r)
	checkUnshareUser(r)
	checkKeyctl(r)
	checkCgroup(r)
	// Last: a process allowed to setuid(0) becomes root and would change the
	// answer to every check after it.
	checkSetuid(r, 0)
	return nil
}
