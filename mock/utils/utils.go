package utils

import (
	"fmt"
	"os"
	"time"
)

const (
	DefaultInterval = 10
)

type Config struct {
	Duration   string `json:"duration"`
	Operations []string `json:"operations"`
	Rates      []int64  `json:"rates"`
}

type Option struct {
	Duration   time.Duration
	Operations []string
	Rates      []int64
	Length     int
}

func Validate(c *Config) {
	_, err := time.ParseDuration(c.Duration)
	if err != nil {
		fmt.Printf("Error: wrong duration: %v\n", c.Duration)
		os.Exit(1)
	}
	if len(c.Operations) != len(c.Rates) {
		fmt.Printf("Error: not equal length.\n")
		os.Exit(1)
	}
}

func NewForConfig(c *Config) *Option {
	d, _ := time.ParseDuration(c.Duration)
	return &Option{
		Duration:   d,
		Operations: c.Operations,
		Rates:      c.Rates,
		Length:     len(c.Operations),
	}
}
