package main

import (
	"fmt"
	"sort"

	"github.com/globalsign/mgo/bson"
	lexicographic_sort "github.com/tolgaOzen/lexicographic-sort"
)

type Tasks []*Task

func (t Tasks) Len() int {
	return len(t)
}

func (t Tasks) Less(i, j int) bool {
	return t[i].Order < t[j].Order
}

func (t Tasks) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

type Task struct {
	ID    string
	Order string
}

func moveElement(slice []*Task, from, to int) []*Task {
	// Check if from and to positions are valid
	if from < 0 || from >= len(slice) || to < 0 || to >= len(slice) {
		fmt.Println("Invalid from/to positions")
		return slice
	}

	// Remove element at from position
	element := slice[from]
	slice = append(slice[:from], slice[from+1:]...)

	// Insert the element at to position
	if to >= len(slice) {
		// If to position is beyond the length of the slice, append the element
		slice = append(slice, element)
	} else {
		// Otherwise, insert the element at to position
		slice = append(slice[:to+1], append([]*Task{element}, slice[to+1:]...)...)
	}

	return slice
}

func moveTask(tasks []*Task, fromIndex, toIndex int) {
	if fromIndex == toIndex {
		return
	}
	if toIndex == len(tasks)-1 {
		tasks[fromIndex].Order = bson.NewObjectId().Hex()
		return
	}

	// change the lexigraphical order
	var before, after string
	if toIndex > 0 {
		before = tasks[toIndex-1].Order
	} else {
		before = " "
	}
	after = tasks[toIndex].Order

	tasks[fromIndex].Order = lexicographic_sort.GenerateBetween(before, after)
	// moveElement(tasks, fromIndex, toIndex)
}

func printTasks(tasks []*Task) {
	for _, task := range tasks {
		fmt.Printf("%s: %s\n", task.ID, task.Order)
	}
}

func main() {
	tasks := []*Task{
		{"A", bson.NewObjectId().Hex()},
		{"B", bson.NewObjectId().Hex()},
		{"C", bson.NewObjectId().Hex()},
	}
	moveTask(tasks, 2, 1)
	sort.Sort(Tasks(tasks))
	printTasks(tasks)
}
