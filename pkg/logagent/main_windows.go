// +build windows

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/utils"
)

var elog debug.Log

const ServiceName = "datadog-log-agent"

// opts are the command-line options
var winopts struct {
	installService   bool
	uninstallService bool
	startService     bool
	stopService      bool
}
var defaultConfigPath = "c:\\programdata\\datadog\\datadog.yaml"
var defaultConfdPath = "c:\\programdata\\datadog\\conf.d"

var ddconfigPath string //= flag.String("ddconfig", "", "Path to the datadog.yaml configuration file")
var ddconfdPath string  //= flag.String("ddconfd", "", "Path to the conf.d directory that contains all integration config files")

func init() {
	flag.StringVar(&ddconfigPath, "ddconfig", defaultConfigPath, "Path to the datadog.yaml configuration file")
	flag.StringVar(&ddconfdPath, "ddconfd", defaultConfdPath, "Path to the conf.d directory that contains all integration config files")

	// windows-specific options for installing the service, uninstalling the service, etc.
	flag.BoolVar(&winopts.installService, "install-service", false, "Install the trace agent to the Service Control Manager")
	flag.BoolVar(&winopts.uninstallService, "uninstall-service", false, "Remove the trace agent from the Service Control Manager")
	flag.BoolVar(&winopts.startService, "start-service", false, "Starts the trace agent service")
	flag.BoolVar(&winopts.stopService, "stop-service", false, "Stops the trace agent service")

	flag.Parse()
}

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	exit := make(chan struct{})

	elog.Info(0x40000003, ServiceName)
	runAgent(exit)

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				elog.Info(0x40000006, ServiceName)
				changes <- svc.Status{State: svc.StopPending}
				///// FIXME:  Need a way to indicate to rest of service to shut
				////  down
				close(exit)
				break
			default:
				elog.Warning(0xc000000A, string(c.Cmd))
			}
		}
	}

	changes <- svc.Status{State: svc.Stopped}
	return
}

func runAgent(exit chan struct{}) {
	utils.SetupLogger()

	err := config.BuildLogsAgentConfig(ddconfigPath, ddconfdPath)
	if err != nil {
		log.Println(err)
		log.Println("Not starting logs-agent")
	} else if config.LogsAgent.GetBool("log_enabled") {
		log.Println("Starting logs-agent")
		Start()

		if config.LogsAgent.GetBool("log_profiling_enabled") {
			log.Println("starting logs-agent profiling")
			go func() {
				log.Println(http.ListenAndServe("localhost:6060", nil))
			}()
		}
	} else {
		log.Println("logs-agent disabled")
	}

}

func runService(isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(ServiceName)
	} else {
		elog, err = eventlog.Open(ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	elog.Info(0x40000007, ServiceName)
	err = run(ServiceName, &myservice{})
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, ServiceName)
}

// main is the main application entry point
func main() {
	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		fmt.Printf("failed to determine if we are running in an interactive session: %v", err)
	}
	if !isIntSess {
		runService(false)
		return
	}
	// sigh.  Go doesn't have boolean xor operator.  The options are mutually exclusive,
	// make sure more than one wasn't specified
	optcount := 0
	if winopts.installService {
		optcount++
	}
	if winopts.uninstallService {
		optcount++
	}
	if winopts.startService {
		optcount++
	}
	if winopts.stopService {
		optcount++
	}
	if optcount > 1 {
		fmt.Printf("Incompatible options chosen")
		return
	}
	if winopts.installService {
		if err = installService(); err != nil {
			fmt.Printf("Error installing service %v\n", err)
		}
		return
	}
	if winopts.uninstallService {
		if err = removeService(); err != nil {
			fmt.Printf("Error removing service %v\n", err)
		}
		return
	}
	if winopts.startService {
		if err = startService(); err != nil {
			fmt.Printf("Error starting service %v\n", err)
		}
		return
	}
	if winopts.stopService {
		if err = stopService(); err != nil {
			fmt.Printf("Error stopping service %v\n", err)
		}
		return

	}

	// if we are an interactive session, then just invoke the agent on the command line.

	exit := make(chan struct{})

	// Invoke the Agent
	runAgent(exit)

	for _ = range exit {

	}
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	err = s.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func stopService() error {
	return controlService(svc.Stop, svc.Stopped)
}

func restartService() error {
	var err error
	if err = stopService(); err == nil {
		err = startService()
	}
	return err
}

func controlService(c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}

func installService() error {
	exepath, err := exePath()
	if err != nil {
		return err
	}
	fmt.Printf("exepath: %s\n", exepath)

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", ServiceName)
	}
	s, err = m.CreateService(ServiceName, exepath, mgr.Config{DisplayName: "Datadog Agent Service"})
	if err != nil {
		return err
	}
	defer s.Close()
	err = eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		s.Delete()
		return fmt.Errorf("SetupEventLogSource() failed: %s", err)
	}
	return nil
}

func exePath() (string, error) {
	prog := os.Args[0]
	p, err := filepath.Abs(prog)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(p)
	if err == nil {
		if !fi.Mode().IsDir() {
			return p, nil
		}
		err = fmt.Errorf("%s is directory", p)
	}
	if filepath.Ext(p) == "" {
		p += ".exe"
		fi, err := os.Stat(p)
		if err == nil {
			if !fi.Mode().IsDir() {
				return p, nil
			}
			err = fmt.Errorf("%s is directory", p)
		}
	}
	return "", err
}

func removeService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", ServiceName)
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	err = eventlog.Remove(ServiceName)
	if err != nil {
		return fmt.Errorf("RemoveEventLogSource() failed: %s", err)
	}
	return nil
}
