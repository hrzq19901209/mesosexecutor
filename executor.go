package main

import (
	"encoding/json"
	"flag"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/go-connections/nat"
	exec "github.com/mesos/mesos-go/executor"
	mesos "github.com/mesos/mesos-go/mesosproto"
	"golang.org/x/net/context"
	"log"
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
	Cpus          float64 `json:"cpus"`
	Mem           float64 `json:"mem"`
	Image         string  `json:"image"`
	ContainerName string  `json:"containerName"`
	Port          string  `json:"port"`
	Sec           int     `json:"sec"`
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
	log.Println("Launching task", taskInfo.GetName(), "with command", taskInfo.Command.GetValue())

	runStatus := &mesos.TaskStatus{
		TaskId: taskInfo.GetTaskId(),
		State:  mesos.TaskState_TASK_STARTING.Enum(),
	}
	_, err := driver.SendStatusUpdate(runStatus)
	if err != nil {
		log.Println("Got error", err)
	}
	e.tasksLaunched++
	log.Println("Total tasls launched ", e.tasksLaunched)

	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := client.NewClient("unix:///var/run/docker.sock", "v1.12", nil, defaultHeaders)
	if err != nil {
		panic(err)
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
		PortBindings: portMap,
		Resources:    resources,
	}

	response, err := cli.ContainerCreate(context.Background(), &container.Config{Image: task.Image}, hostConfig, nil, task.ContainerName)

	if err != nil {
		panic(err)
	}
	cli.ContainerStart(context.Background(), response.ID, types.ContainerStartOptions{})

	runStatus = &mesos.TaskStatus{
		TaskId: taskInfo.GetTaskId(),
		State:  mesos.TaskState_TASK_RUNNING.Enum(),
	}
	_, err = driver.SendStatusUpdate(runStatus)
	if err != nil {
		log.Println("Got error", err)
	}

	cli.ContainerWait(context.Background(), response.ID)
	//cli.ContainerRemove(context.Background(), response.ID, types.ContainerRemoveOptions{})
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

func main() {
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
