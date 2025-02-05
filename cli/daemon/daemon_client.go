package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
)

const (
	defaultDaemonAddr = "http://127.0.0.1:31010"
)

// SendTaskStart sends a start request for a given task.
func SendTaskStart(task string, params map[string]string) (*TaskResponse, error) {
	reqData := TaskRequest{Task: task, Params: params}
	b, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(defaultDaemonAddr+"/task/start", "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var taskResp TaskResponse
	err = json.Unmarshal(body, &taskResp)
	if err != nil {
		return nil, err
	}
	if taskResp.Status == "error" {
		return &taskResp, errors.New(taskResp.Error)
	}
	return &taskResp, nil
}

// SendTaskStop sends a stop request for a given task.
func SendTaskStop(task string, params map[string]string) (*TaskResponse, error) {
	reqData := TaskRequest{Task: task, Params: params}
	b, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(defaultDaemonAddr+"/task/stop", "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var taskResp TaskResponse
	err = json.Unmarshal(body, &taskResp)
	if err != nil {
		return nil, err
	}
	if taskResp.Status == "error" {
		return &taskResp, errors.New(taskResp.Error)
	}
	return &taskResp, nil

}

// SendTaskStatus sends a status request for a given task.
func SendTaskStatus(task string) (*TaskResponse, error) {
	resp, err := http.Get(defaultDaemonAddr + "/task/status?task=" + task)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var taskResp TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return nil, err
	}
	if taskResp.Status == "error" {
		return &taskResp, errors.New(taskResp.Error)
	}
	return &taskResp, nil
}
