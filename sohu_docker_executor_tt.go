package main

import (
	"encoding/json"
	"flag"
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
	log.Println("Registered Executor on slave ", slaveInfo.GetHostname())
}

func (e *svcExecutor) Reregistered(driver exec.ExecutorDriver, slaveInfo *mesos.SlaveInfo) {
	log.Println("Re-registered Executor on slave ", slaveInfo.GetHostname())
}

func (e *svcExecutor) Disconnected(exec.ExecutorDriver) {
	log.Println("Executor disconnected.")
}

func (e *svcExecutor) LaunchTask(driver exec.ExecutorDriver, taskInfo *mesos.TaskInfo) {
	go runTask(driver, taskInfo)
	e.tasksLaunched++
	log.Println("Total tasls launched ", e.tasksLaunched)
}
func runTask(driver exec.ExecutorDriver, taskInfo *mesos.TaskInfo) {
	log.Println("Launching task", taskInfo.GetName(), "with command", taskInfo.Command.GetValue())

	runStatus := &mesos.TaskStatus{
		TaskId: taskInfo.GetTaskId(),
		State:  mesos.TaskState_TASK_STARTING.Enum(),
	}
	_, err := driver.SendStatusUpdate(runStatus)
	if err != nil {
		log.Println("Got error", err)
	}

	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := client.NewClient("unix:///var/run/docker.sock", "v1.12", nil, defaultHeaders)
	if err != nil {
		log.Println(err)
		return
	}

	var task Task
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
		log.Println(err)
		return
	}

	cli.ContainerStart(context.Background(), response.ID, types.ContainerStartOptions{})

	runStatus = &mesos.TaskStatus{
		TaskId: taskInfo.GetTaskId(),
		State:  mesos.TaskState_TASK_RUNNING.Enum(),
	}
	_, err = driver.SendStatusUpdate(runStatus)
	if err != nil {
		log.Println("Got error", err)
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
		log.Println("Got error", err)
		return
	}

	log.Println("Task finished", taskInfo.GetName())
}

func (e *svcExecutor) KillTask(exec.ExecutorDriver, *mesos.TaskID) {
	log.Println("Kill task")
}

func (e *svcExecutor) FrameworkMessage(driver exec.ExecutorDriver, msg string) {
	log.Println("Got framework message: ", msg)
}

func (e *svcExecutor) Shutdown(exec.ExecutorDriver) {
	log.Println("Shutting down the executor")
}

func (e *svcExecutor) Error(driver exec.ExecutorDriver, err string) {
	log.Println("Got error message:", err)
}

func init() {
	flag.Parse()
}

func checkFileExist(file string) bool {
	exist := true
	if _, err := os.Stat(file); os.IsNotExist(err) {
		exist = false
	}
	return exist
}
func main() {
	fileName := "/var/log/sohu-docker-executor.log"
	var logFile *os.File
	var err error
	if !checkFileExist(fileName) {
		logFile, err = os.Create(fileName)
		if err != nil {
			panic(err)
		}
	}
	logFile, err = os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	log.Println("Starting Example Executor (Go)")

	dconfig := exec.DriverConfig{
		Executor: newSvcExecutor(),
	}
	driver, err := exec.NewMesosExecutorDriver(dconfig)

	if err != nil {
		log.Println("Unable to create a ExecutorDriver ", err.Error())
	}

	_, err = driver.Start()
	if err != nil {
		log.Println("Got error:", err)
		return
	}
	log.Println("Executor process has started and running.")
	driver.Join()
}
