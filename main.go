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

type ValidationError struct {
	Field   string
	Message string
	Line    int
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Функция для получения номера строки (упрощенная версия)
func getLineNumber(data []byte, field string) int {
	// Простая реализация - в реальном приложении нужно использовать более сложную логику
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.Contains(line, field+":") {
			return i + 1
		}
	}
	return 0
}

func validatePod(pod *Pod, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	// Валидация верхнего уровня
	if pod.APIVersion != "v1" {
		line := getLineNumber(data, "apiVersion")
		errors = append(errors, ValidationError{filename, "apiVersion must be 'v1'", line})
	}

	if pod.Kind != "Pod" {
		line := getLineNumber(data, "kind")
		errors = append(errors, ValidationError{filename, "kind must be 'Pod'", line})
	}

	// Валидация metadata
	errors = append(errors, validateObjectMeta(&pod.Metadata, data, filename)...)

	// Валидация spec
	errors = append(errors, validatePodSpec(&pod.Spec, data, filename)...)

	return errors
}

func validateObjectMeta(meta *ObjectMeta, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	if meta.Name == "" {
		line := getLineNumber(data, "name")
		errors = append(errors, ValidationError{filename, "name is required", line})
	}

	return errors
}

func validatePodSpec(spec *PodSpec, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	// Валидация OS
	if spec.OS != "" && spec.OS != "linux" && spec.OS != "windows" {
		line := getLineNumber(data, "os")
		errors = append(errors, ValidationError{filename, fmt.Sprintf("os has unsupported value '%s'", spec.OS), line})
	}

	// Валидация containers
	if len(spec.Containers) == 0 {
		line := getLineNumber(data, "containers")
		errors = append(errors, ValidationError{filename, "at least one container is required", line})
	} else {
		for i, container := range spec.Containers {
			containerErrors := validateContainer(&container, i, data, filename)
			errors = append(errors, containerErrors...)
		}
	}

	return errors
}

func validateContainer(container *Container, index int, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	// Валидация name
	if container.Name == "" {
		line := getLineNumber(data, "name")
		errors = append(errors, ValidationError{filename, "container name is required", line})
	} else {
		// Проверка формата snake_case
		snakeCaseRegex := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
		if !snakeCaseRegex.MatchString(container.Name) {
			line := getLineNumber(data, "name")
			errors = append(errors, ValidationError{filename, "container name must be in snake_case format", line})
		}
	}

	// Валидация image
	if container.Image == "" {
		line := getLineNumber(data, "image")
		errors = append(errors, ValidationError{filename, "image is required", line})
	} else {
		// Проверка домена и тега
		if !strings.HasPrefix(container.Image, "registry.bigbrother.io/") {
			line := getLineNumber(data, "image")
			errors = append(errors, ValidationError{filename, "image must be in domain registry.bigbrother.io", line})
		}
		
		parts := strings.Split(container.Image, ":")
		if len(parts) != 2 || parts[1] == "" {
			line := getLineNumber(data, "image")
			errors = append(errors, ValidationError{filename, "image must have version tag", line})
		}
	}

	// Валидация ports
	for i, port := range container.Ports {
		portErrors := validateContainerPort(&port, i, data, filename)
		errors = append(errors, portErrors...)
	}

	// Валидация probes
	if container.ReadinessProbe != nil {
		probeErrors := validateProbe(container.ReadinessProbe, "readinessProbe", data, filename)
		errors = append(errors, probeErrors...)
	}

	if container.LivenessProbe != nil {
		probeErrors := validateProbe(container.LivenessProbe, "livenessProbe", data, filename)
		errors = append(errors, probeErrors...)
	}

	// Валидация resources
	errors = append(errors, validateResourceRequirements(&container.Resources, data, filename)...)

	return errors
}

func validateContainerPort(port *ContainerPort, index int, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	// Валидация containerPort
	if port.ContainerPort <= 0 || port.ContainerPort >= 65536 {
		line := getLineNumber(data, "containerPort")
		errors = append(errors, ValidationError{filename, "containerPort value out of range", line})
	}

	// Валидация protocol
	if port.Protocol != "" && port.Protocol != "TCP" && port.Protocol != "UDP" {
		line := getLineNumber(data, "protocol")
		errors = append(errors, ValidationError{filename, "protocol must be 'TCP' or 'UDP'", line})
	}

	return errors
}

func validateProbe(probe *Probe, probeType string, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	// Валидация httpGet
	errors = append(errors, validateHTTPGetAction(&probe.HTTPGet, probeType, data, filename)...)

	return errors
}

func validateHTTPGetAction(action *HTTPGetAction, probeType string, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	if action.Path == "" {
		line := getLineNumber(data, "path")
		errors = append(errors, ValidationError{filename, "path is required", line})
	} else if !strings.HasPrefix(action.Path, "/") {
		line := getLineNumber(data, "path")
		errors = append(errors, ValidationError{filename, "path must be absolute", line})
	}

	if action.Port <= 0 || action.Port >= 65536 {
		line := getLineNumber(data, "port")
		errors = append(errors, ValidationError{filename, "port value out of range", line})
	}

	return errors
}

func validateResourceRequirements(resources *ResourceRequirements, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	// Валидация requests
	if resources.Requests != nil {
		requestErrors := validateResourceMap(resources.Requests, "requests", data, filename)
		errors = append(errors, requestErrors...)
	}

	// Валидация limits
	if resources.Limits != nil {
		limitErrors := validateResourceMap(resources.Limits, "limits", data, filename)
		errors = append(errors, limitErrors...)
	}

	return errors
}

func validateResourceMap(resourceMap map[string]interface{}, mapType string, data []byte, filename string) []ValidationError {
	var errors []ValidationError

	for resource, value := range resourceMap {
		switch resource {
		case "cpu":
			switch v := value.(type) {
			case int:
				if v <= 0 {
					line := getLineNumber(data, "cpu")
					errors = append(errors, ValidationError{filename, "cpu must be positive integer", line})
				}
			case string:
				if _, err := strconv.Atoi(v); err != nil {
					line := getLineNumber(data, "cpu")
					errors = append(errors, ValidationError{filename, "cpu must be int", line})
				}
			default:
				line := getLineNumber(data, "cpu")
				errors = append(errors, ValidationError{filename, "cpu must be integer", line})
			}

		case "memory":
			if memory, ok := value.(string); ok {
				if !isValidMemoryValue(memory) {
					line := getLineNumber(data, "memory")
					errors = append(errors, ValidationError{filename, "memory must be in format with Gi, Mi, or Ki suffix", line})
				}
			} else {
				line := getLineNumber(data, "memory")
				errors = append(errors, ValidationError{filename, "memory must be string", line})
			}

		default:
			line := getLineNumber(data, resource)
			errors = append(errors, ValidationError{filename, "unknown resource type", line})
		}
	}

	return errors
}

func isValidMemoryValue(memory string) bool {
	// Проверка формата: число + суффикс Gi, Mi, Ki
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

	// Валидация
	validationErrors := validatePod(&pod, data, filename)

	if len(validationErrors) > 0 {
		for _, err := range validationErrors {
			if err.Line > 0 {
				fmt.Fprintf(os.Stderr, "%s:%d %s\n", err.Field, err.Line, err.Message)
			} else {
				fmt.Fprintf(os.Stderr, "%s %s\n", err.Field, err.Message)
			}
		}
		os.Exit(1)
	}

	fmt.Println("YAML configuration is valid!")
	os.Exit(0)
}