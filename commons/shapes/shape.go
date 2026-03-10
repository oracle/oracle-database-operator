package shapes

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ShapeConfig struct {
	CPU       int
	SGAGB     int
	PGAGB     int
	Processes int
}

var shapeTable = map[string]ShapeConfig{
	"kodb1":  {CPU: 1, SGAGB: 4, PGAGB: 2, Processes: 200},
	"kodb2":  {CPU: 2, SGAGB: 8, PGAGB: 4, Processes: 400},
	"kodb4":  {CPU: 4, SGAGB: 16, PGAGB: 8, Processes: 800},
	"kodb6":  {CPU: 6, SGAGB: 24, PGAGB: 12, Processes: 1200},
	"kodb12": {CPU: 12, SGAGB: 48, PGAGB: 24, Processes: 2400},
	"kodb16": {CPU: 16, SGAGB: 64, PGAGB: 32, Processes: 3200},
	"kodb24": {CPU: 24, SGAGB: 96, PGAGB: 48, Processes: 4800},
	"kodb32": {CPU: 32, SGAGB: 128, PGAGB: 64, Processes: 6400},
	"kodb36": {CPU: 36, SGAGB: 128, PGAGB: 64, Processes: 7200},
}

func LookupShapeConfig(shape string) (ShapeConfig, bool) {
	cfg, ok := shapeTable[strings.ToLower(strings.TrimSpace(shape))]
	return cfg, ok
}

func (c ShapeConfig) totalMemGi() int {
	return c.SGAGB + c.PGAGB + 1
}

func (c ShapeConfig) sgaMB() int {
	return c.SGAGB * 1024
}

func (c ShapeConfig) pgaMB() int {
	return c.PGAGB * 1024
}

func (c ShapeConfig) totalMB() int {
	return c.totalMemGi() * 1024
}

func (c ShapeConfig) EnvPairs() [][2]string {
	return [][2]string{
		{"INIT_SGA_SIZE", fmt.Sprintf("%d", c.sgaMB())},
		{"INIT_PGA_SIZE", fmt.Sprintf("%d", c.pgaMB())},
		{"INIT_PROCESS", fmt.Sprintf("%d", c.Processes)},
		{"INIT_CPU_COUNT", fmt.Sprintf("%d", c.CPU)},
		{"INIT_TOTAL_SIZE", fmt.Sprintf("%d", c.totalMB())},
	}
}

func (c ShapeConfig) ResourceRequirements() *corev1.ResourceRequirements {
	cpuQ := resource.MustParse(fmt.Sprintf("%d", c.CPU))
	memQ := resource.MustParse(fmt.Sprintf("%dGi", c.totalMemGi()))
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    cpuQ,
			corev1.ResourceMemory: memQ,
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    cpuQ,
			corev1.ResourceMemory: memQ,
		},
	}
}
