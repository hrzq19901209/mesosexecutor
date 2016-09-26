package main

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/strslice"
	"github.com/docker/go-connections/nat"
	exec "github.com/mesos/mesos-go/executor"
	mesos "github.com/mesos/mesos-go/mesosproto"
	"golang.org/x/net/context"
	"os"
)

type svcExecutor struct {
	tasksLaunched int
}

func newSvcExecutor() *svcExecutor {
	return &svcExecutor{
		tasksLaunched: 0,
	}
}

type Task struct {
	Cpus          float64  `json:"cpus"`
	Mem           float64  `json:"mem"`
	Image         string   `json:"image"`
	ContainerName string   `json:"containerName"`
	Port          string   `json:"port"`
	Volume        []string `json:"volume"`
	NetworkMode   string   `json:"networkmode"`
	Cmd           []string `json:"cmd"`
}

func (e *svcExecutor) Registered(driver exec.ExecutorDriver, execInfo *mesos.ExecutorInfo, fwinfo *mesos.FrameworkInfo, slaveInfo *mesos.SlaveInfo) {
	log.Info(fmt.Sprintf("Registered Executor on slave %s ", slaveInfo.GetHostname()))
}

func (e *svcExecutor) Reregistered(driver exec.ExecutorDriver, slaveInfo *mesos.SlaveInfo) {
	log.Info(fmt.Sprintf("Re-registered Executor on slave %s", slaveInfo.GetHostname()))
}

func (e *svcExecutor) Disconnected(exec.ExecutorDriver) {
	log.Info("Executor disconnected.")
}

func (e *svcExecutor) LaunchTask(driver exec.ExecutorDriver, taskInfo *mesos.TaskInfo) {
	go runTask(driver, taskInfo)
	e.tasksLaunched++
	log.Info(fmt.Sprintf("Total tasls launched %d", e.tasksLaunched))
}

func runTask(driver exec.ExecutorDriver, taskInfo *mesos.TaskInfo) {
	log.Info(fmt.Sprintf("Launching task %s with command %s ", taskInfo.GetName(), taskInfo.Command.GetValue()))

	runStatus := &mesos.TaskStatus{
		TaskId: taskInfo.GetTaskId(),
		State:  mesos.TaskState_TASK_STARTING.Enum(),
	}
	_, err := driver.SendStatusUpdate(runStatus)
	if err != nil {
		log.Error(fmt.Sprintf("Got error %s", err))
		return
	}

	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := client.NewClient("unix:///var/run/docker.sock", "v1.12", nil, defaultHeaders)
	if err != nil {
		log.Error(err)
		return
	}

	var task Task
	log.Info(string(taskInfo.Data))
	json.Unmarshal(taskInfo.Data, &task)

	portMap := make(nat.PortMap)
	portBinding80 := nat.PortBinding{
		HostIP:   "0.0.0.0",
		HostPort: task.Port,
	}
	bingArray := []nat.PortBinding{portBinding80}
	portMap["80/tcp"] = bingArray

	resources := container.Resources{
		CPUQuota:  int64(task.Cpus) * 100000,
		Memory:    int64(task.Mem) * 1024 * 1024,
		CPUPeriod: 100000,
	}

	hostConfig := &container.HostConfig{
		//PortBindings: portMap,
		NetworkMode: container.NetworkMode(task.NetworkMode),
		Resources:   resources,
		Binds:       task.Volume,
	}

	var cmd strslice.StrSlice = task.Cmd
	config := &container.Config{
		Image: task.Image,
		Cmd:   cmd,
	}

	response, err := cli.ContainerCreate(context.Background(), config, hostConfig, nil, task.ContainerName)

	if err != nil {
		log.Error(err)
		return
	}

	cli.ContainerStart(context.Background(), response.ID, types.ContainerStartOptions{})

	runStatus = &mesos.TaskStatus{
		TaskId: taskInfo.GetTaskId(),
		State:  mesos.TaskState_TASK_RUNNING.Enum(),
	}
	_, err = driver.SendStatusUpdate(runStatus)
	if err != nil {
		log.Error("Got error %s", err)
		return
	}

	cli.ContainerWait(context.Background(), response.ID)
	cli.ContainerRemove(context.Background(), response.ID, types.ContainerRemoveOptions{})
	// Finish task
	finStatus := &mesos.TaskStatus{
		TaskId: taskInfo.GetTaskId(),
		State:  mesos.TaskState_TASK_FINISHED.Enum(),
	}
	_, err = driver.SendStatusUpdate(finStatus)
	if err != nil {
		log.Error(fmt.Sprintf("Got error %s", err))
		return
	}

	log.Info(fmt.Sprintf("Task finished %s", taskInfo.GetName()))
}

func (e *svcExecutor) KillTask(exec.ExecutorDriver, *mesos.TaskID) {
	log.Info("Kill task")
}

func (e *svcExecutor) FrameworkMessage(driver exec.ExecutorDriver, msg string) {
	log.Info(fmt.Sprintf("Got framework message %s ", msg))
}

func (e *svcExecutor) Shutdown(exec.ExecutorDriver) {
	log.Info("Shutting down the executor")
}

func (e *svcExecutor) Error(driver exec.ExecutorDriver, err string) {
	log.Info(fmt.Sprintf("Got error message %s", err))
}

func checkFileExist(file string) bool {
	exist := true
	if _, err := os.Stat(file); os.IsNotExist(err) {
		exist = false
	}
	return exist
}

func init() {
	fileName := "/var/log/sohu-docker-executor.log"
	var logFile *os.File
	var err error
	if !checkFileExist(fileName) {
		logFile, err = os.Create(fileName)
		if err != nil {
			log.Error(fmt.Sprintf("create log file %s error %s", fileName, err))
			return
		}
	}
	logFile, err = os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Error(fmt.Sprintf("open log file %s error %s", fileName, err))
		return
	}
	//defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: "2016-09-20 16:06:04",
		FullTimestamp:   true,
		ForceColors:     true,
	})
	log.SetLevel(log.InfoLevel)
}

func main() {
	log.Info("Starting Example Executor (Go)")
	config := exec.DriverConfig{
		Executor: newSvcExecutor(),
	}
	driver, err := exec.NewMesosExecutorDriver(config)

	if err != nil {
		log.Error(fmt.Sprintf("Unable to create a ExecutorDriver %s", err))
		return
	}

	_, err = driver.Start()
	if err != nil {
		log.Error(fmt.Sprintf("Got error %s", err))
		return
	}
	log.Info("Executor process has started and running.")
	driver.Join()
	log.Info("Executor process has stopped.")
}
