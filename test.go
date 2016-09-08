package main

import (
	"encoding/json"
	"fmt"
)

type Task struct {
	Volume []string `json:"volume"`
}

func main() {
	jsonBody := []byte(`{"volume":["/var/log/server:/var/log/server","/opt/haoran:/opt/haoran"]}`)
	var task Task
	json.Unmarshal(jsonBody, &task)
	for _, v := range task.Volume {
		fmt.Println(v)
	}
}
