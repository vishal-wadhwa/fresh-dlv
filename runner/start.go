package runner

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var (
	startChannel chan string
	stopChannel  chan bool
	mainLog      logFunc
	watcherLog   logFunc
	runnerLog    logFunc
	buildLog     logFunc
	appLog       logFunc
	debuggerLog  logFunc
)

func flushEvents() {
	for {
		select {
		case eventName := <-startChannel:
			mainLog("receiving event %s", eventName)
		default:
			return
		}
	}
}

func start() {
	loopIndex := 0
	buildDelay := buildDelay()

	started := false
	var killAllFn KillFn
	go func() {
		<-stopChannel
		if killAllFn != nil {
			killAllFn()
		}
	}()

	go func() {
		for {
			loopIndex++
			mainLog("Waiting (loop %d)...", loopIndex)
			eventName := <-startChannel

			mainLog("receiving first event %s", eventName)
			mainLog("sleeping for %d milliseconds", buildDelay)
			time.Sleep(buildDelay * time.Millisecond)
			mainLog("flushing events")

			flushEvents()

			mainLog("Started! (%d Goroutines)", runtime.NumGoroutine())
			err := removeBuildErrorsLog()
			if err != nil {
				mainLog("%s", err.Error())
			}

			debug := isDebuggingEnabled()
			buildFailed := false
			if shouldRebuild(eventName) {
				errorMessage, ok := build(debug)
				if !ok {
					buildFailed = true
					mainLog("Build Failed: \n %s", errorMessage)
					if !started {
						os.Exit(1)
					}
					createBuildErrorsLog(errorMessage)
				}
			}

			if !buildFailed {
				if started {
					killAllFn()
				}
				killAllFn = run(debug)
			}

			started = true
			mainLog("%s", strings.Repeat("-", 20))
		}
	}()
}

func init() {
	startChannel = make(chan string, 1000)
	stopChannel = make(chan bool)
}

func initSignalTraps() {
	sigIntChannel := make(chan os.Signal, 1)
	signal.Notify(sigIntChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigIntChannel
		mainLog("sigint received - killing child processes - [waiting for 2 seconds]")
		time.AfterFunc(time.Second*2, func() {
			mainLog("done waiting :)")
			os.Exit(1)
		})

		stopChannel <- true
	}()

}

func initLogFuncs() {
	mainLog = newLogFunc("main")
	watcherLog = newLogFunc("watcher")
	runnerLog = newLogFunc("runner")
	buildLog = newLogFunc("build")
	appLog = newLogFunc("app")
	debuggerLog = newLogFunc("debugger")
}

func setEnvVars() {
	os.Setenv("DEV_RUNNER", "1")
	wd, err := os.Getwd()
	if err == nil {
		os.Setenv("RUNNER_WD", wd)
	}

	for k, v := range settings {
		key := strings.ToUpper(fmt.Sprintf("%s%s", envSettingsPrefix, k))
		os.Setenv(key, v)
	}
}

// Watches for file changes in the root directory.
// After each file system event it builds and (re)starts the application.
func Start() {
	initLimit()
	initSettings()
	initLogFuncs()
	initFolders()
	initSignalTraps()
	setEnvVars()
	watch()
	start()
	startChannel <- "/"

	<-make(chan int)
}
