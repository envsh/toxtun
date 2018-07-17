package main

import (
	"sync"
	"time"
)

type TimedCounter struct {
	mu    sync.Mutex
	times map[string][]time.Time
}

var timc = &TimedCounter{times: map[string][]time.Time{}}

func (this *TimedCounter) Mark(name string) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.times[name] = append(this.times[name], time.Now())
	this.truncate(name)
}

// 1min
func (this *TimedCounter) truncate(name string) {
	nowt := time.Now()
	for i := 0; i < len(this.times[name]); i++ {
		if int(nowt.Sub(this.times[name][i]).Seconds()) > 60 {
			continue
		}
		this.times[name] = this.times[name][i:]
		break
	}
}

func (this *TimedCounter) Count1(name string) int {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.truncate(name)
	return len(this.times[name])
}

func (this *TimedCounter) Count5() int {
	return 0
}

func (this *TimedCounter) Count15() int {
	return 0
}
