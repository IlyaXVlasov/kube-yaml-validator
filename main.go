package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Структуры для декодирования YAML
type Pod struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   ObjectMeta `yaml:"metadata"`
	Spec       PodSpec    `yaml:"spec"`
}

type ObjectMeta struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

type PodSpec struct {
	OS        string      `yaml:"os,omitempty"`
	Containers []Container `yaml:"containers"`
}

type Container struct {
	Name          string               `yaml:"name"`
	Image         string               `yaml:"image"`
	Ports         []ContainerPort      `yaml:"ports,omitempty"`
	ReadinessProbe *Probe              `yaml:"readinessProbe,omitempty"`
	LivenessProbe  *Probe              `yaml:"livenessProbe,omitempty"`
	Resources     ResourceRequirements `yaml:"resources"`
}

type ContainerPort struct {
	ContainerPort int    `yaml:"containerPort"`
	Protocol      string `yaml:"protocol,omitempty"`
}

type Probe struct {
	HTTPGet HTTPGetAction `yaml:"httpGet"`
}

type HTTPGetAction struct {
	Path string `yaml:"path"`
	Port int    `yaml:"port"`
}

type ResourceRequirements struct {
	Requests map[string]interface{} `yaml:"requests,omitempty"`
	Limits   map[string]interface{} `yaml:"limits,omitempty"`
}

// Валидаторы
func validatePod(pod *Pod, filename string) []string {
	var errors []string

	// Валидация верхнего уровня
	if pod.APIVersion != "v1" {
		errors = append(errors, fmt.Sprintf("%s:4 apiVersion must be 'v1'", filename))
	}

	if pod.Kind != "Pod" {
		errors = append(errors, fmt.Sprintf("%s:2 kind must be 'Pod'", filename))
	}

	// Валидация metadata
	if pod.Metadata.Name == "" {
		errors = append(errors, fmt.Sprintf("%s:4 name is required", filename))
	}

	// Валидация spec OS
	if pod.Spec.OS != "" && pod.Spec.OS != "linux" && pod.Spec.OS != "windows" {
		errors = append(errors, fmt.Sprintf("%s:10 os has unsupported value '%s'", filename, pod.Spec.OS))
	}

	// Валидация containers
	if len(pod.Spec.Containers) == 0 {
		errors = append(errors, fmt.Sprintf("%s:12 at least one container is required", filename))
	} else {
		for i, container := range pod.Spec.Containers {
			containerErrors := validateContainer(&container, i, filename)
			errors = append(errors, containerErrors...)
		}
	}

	return errors
}

func validateContainer(container *Container, index int, filename string) []string {
	var errors []string
	lineOffset := 12 + index * 20 // Примерное вычисление строки

	// Валидация name
	if container.Name == "" {
		errors = append(errors, fmt.Sprintf("%s:%d container name is required", filename, lineOffset+1))
	} else {
		snakeCaseRegex := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
		if !snakeCaseRegex.MatchString(container.Name) {
			errors = append(errors, fmt.Sprintf("%s:%d container name must be in snake_case format", filename, lineOffset+1))
		}
	}

	// Валидация image
	if container.Image == "" {
		errors = append(errors, fmt.Sprintf("%s:%d image is required", filename, lineOffset+2))
	} else {
		if !strings.HasPrefix(container.Image, "registry.bigbrother.io/") {
			errors = append(errors, fmt.Sprintf("%s:%d image must be in domain registry.bigbrother.io", filename, lineOffset+2))
		}
		
		parts := strings.Split(container.Image, ":")
		if len(parts) != 2 || parts[1] == "" {
			errors = append(errors, fmt.Sprintf("%s:%d image must have version tag", filename, lineOffset+2))
		}
	}

	// Валидация ports
	for i, port := range container.Ports {
		portErrors := validateContainerPort(&port, i, filename, lineOffset+4+i)
		errors = append(errors, portErrors...)
	}

	// Валидация probes
	if container.ReadinessProbe != nil {
		probeErrors := validateProbe(container.ReadinessProbe, "readinessProbe", filename, lineOffset+8)
		errors = append(errors, probeErrors...)
	}

	if container.LivenessProbe != nil {
		probeErrors := validateProbe(container.LivenessProbe, "livenessProbe", filename, lineOffset+12)
		errors = append(errors, probeErrors...)
	}

	// Валидация resources
	resourceErrors := validateResourceRequirements(&container.Resources, filename, lineOffset+16)
	errors = append(errors, resourceErrors...)

	return errors
}

func validateContainerPort(port *ContainerPort, index int, filename string, line int) []string {
	var errors []string

	if port.ContainerPort <= 0 || port.ContainerPort >= 65536 {
		errors = append(errors, fmt.Sprintf("%s:%d containerPort value out of range", filename, line))
	}

	if port.Protocol != "" && port.Protocol != "TCP" && port.Protocol != "UDP" {
		errors = append(errors, fmt.Sprintf("%s:%d protocol must be 'TCP' or 'UDP'", filename, line+1))
	}

	return errors
}

func validateProbe(probe *Probe, probeType string, filename string, line int) []string {
	var errors []string

	if probe.HTTPGet.Path == "" {
		errors = append(errors, fmt.Sprintf("%s:%d path is required", filename, line+1))
	} else if !strings.HasPrefix(probe.HTTPGet.Path, "/") {
		errors = append(errors, fmt.Sprintf("%s:%d path must be absolute", filename, line+1))
	}

	if probe.HTTPGet.Port <= 0 || probe.HTTPGet.Port >= 65536 {
		errors = append(errors, fmt.Sprintf("%s:%d port value out of range", filename, line+2))
	}

	return errors
}

func validateResourceRequirements(resources *ResourceRequirements, filename string, line int) []string {
	var errors []string

	// Валидация limits
	if resources.Limits != nil {
		limitErrors := validateResourceMap(resources.Limits, "limits", filename, line+1)
		errors = append(errors, limitErrors...)
	}

	// Валидация requests
	if resources.Requests != nil {
		requestErrors := validateResourceMap(resources.Requests, "requests", filename, line+4)
		errors = append(errors, requestErrors...)
	}

	return errors
}

func validateResourceMap(resourceMap map[string]interface{}, mapType string, filename string, line int) []string {
	var errors []string

	for resource, value := range resourceMap {
		switch resource {
		case "cpu":
			switch v := value.(type) {
			case int:
				if v <= 0 {
					errors = append(errors, fmt.Sprintf("%s:%d cpu must be positive integer", filename, line))
				}
			case string:
				if _, err := strconv.Atoi(v); err != nil {
					errors = append(errors, fmt.Sprintf("%s:%d cpu must be int", filename, line))
				}
			default:
				errors = append(errors, fmt.Sprintf("%s:%d cpu must be integer", filename, line))
			}

		case "memory":
			if memory, ok := value.(string); ok {
				if !isValidMemoryValue(memory) {
					errors = append(errors, fmt.Sprintf("%s:%d memory must be in format with Gi, Mi, or Ki suffix", filename, line+1))
				}
			} else {
				errors = append(errors, fmt.Sprintf("%s:%d memory must be string", filename, line+1))
			}
		}
	}

	return errors
}

func isValidMemoryValue(memory string) bool {
	memoryRegex := regexp.MustCompile(`^\d+(Gi|Mi|Ki)$`)
	return memoryRegex.MatchString(memory)
}

// Основная логика приложения
func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <yaml-file>\n", os.Args[0])
		os.Exit(1)
	}

	filename := os.Args[1]

	// Чтение файла
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Декодирование YAML
	var pod Pod
	if err := yaml.Unmarshal(data, &pod); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	// Отладочный вывод
	fmt.Fprintf(os.Stderr, "DEBUG: Name='%s', OS='%s', Containers=%d\n", 
		pod.Metadata.Name, pod.Spec.OS, len(pod.Spec.Containers))

	// Валидация
	validationErrors := validatePod(&pod, filename)

	if len(validationErrors) > 0 {
		for _, err := range validationErrors {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}

	fmt.Println("YAML configuration is valid!")
	os.Exit(0)
}