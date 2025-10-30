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
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func validatePod(pod *Pod) []ValidationError {
	var errors []ValidationError

	// Валидация верхнего уровня
	if pod.APIVersion != "v1" {
		errors = append(errors, ValidationError{"apiVersion", "must be 'v1'"})
	}

	if pod.Kind != "Pod" {
		errors = append(errors, ValidationError{"kind", "must be 'Pod'"})
	}

	// Валидация metadata
	errors = append(errors, validateObjectMeta(&pod.Metadata)...)

	// Валидация spec
	errors = append(errors, validatePodSpec(&pod.Spec)...)

	return errors
}

func validateObjectMeta(meta *ObjectMeta) []ValidationError {
	var errors []ValidationError

	if meta.Name == "" {
		errors = append(errors, ValidationError{"metadata.name", "is required"})
	}

	return errors
}

func validatePodSpec(spec *PodSpec) []ValidationError {
	var errors []ValidationError

	// Валидация OS (теперь как string)
	if spec.OS != "" && spec.OS != "linux" && spec.OS != "windows" {
		errors = append(errors, ValidationError{"spec.os", "must be 'linux' or 'windows'"})
	}

	// Валидация containers
	if len(spec.Containers) == 0 {
		errors = append(errors, ValidationError{"spec.containers", "at least one container is required"})
	} else {
		for i, container := range spec.Containers {
			containerErrors := validateContainer(&container, i)
			errors = append(errors, containerErrors...)
		}
	}

	return errors
}

func validateContainer(container *Container, index int) []ValidationError {
	var errors []ValidationError
	containerPrefix := fmt.Sprintf("spec.containers[%d]", index)

	// Валидация name
	if container.Name == "" {
		errors = append(errors, ValidationError{containerPrefix + ".name", "is required"})
	} else {
		// Проверка формата snake_case
		snakeCaseRegex := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
		if !snakeCaseRegex.MatchString(container.Name) {
			errors = append(errors, ValidationError{containerPrefix + ".name", "must be in snake_case format"})
		}
	}

	// Валидация image
	if container.Image == "" {
		errors = append(errors, ValidationError{containerPrefix + ".image", "is required"})
	} else {
		// Проверка домена и тега
		if !strings.HasPrefix(container.Image, "registry.bigbrother.io/") {
			errors = append(errors, ValidationError{containerPrefix + ".image", "must be in domain registry.bigbrother.io"})
		}
		
		parts := strings.Split(container.Image, ":")
		if len(parts) != 2 || parts[1] == "" {
			errors = append(errors, ValidationError{containerPrefix + ".image", "must have version tag"})
		}
	}

	// Валидация ports
	for i, port := range container.Ports {
		portErrors := validateContainerPort(&port, i, containerPrefix)
		errors = append(errors, portErrors...)
	}

	// Валидация probes
	if container.ReadinessProbe != nil {
		probeErrors := validateProbe(container.ReadinessProbe, "readinessProbe", containerPrefix)
		errors = append(errors, probeErrors...)
	}

	if container.LivenessProbe != nil {
		probeErrors := validateProbe(container.LivenessProbe, "livenessProbe", containerPrefix)
		errors = append(errors, probeErrors...)
	}

	// Валидация resources
	errors = append(errors, validateResourceRequirements(&container.Resources, containerPrefix)...)

	return errors
}

func validateContainerPort(port *ContainerPort, index int, containerPrefix string) []ValidationError {
	var errors []ValidationError
	portPrefix := fmt.Sprintf("%s.ports[%d]", containerPrefix, index)

	// Валидация containerPort
	if port.ContainerPort <= 0 || port.ContainerPort >= 65536 {
		errors = append(errors, ValidationError{portPrefix + ".containerPort", "must be in range 0 < x < 65536"})
	}

	// Валидация protocol
	if port.Protocol != "" && port.Protocol != "TCP" && port.Protocol != "UDP" {
		errors = append(errors, ValidationError{portPrefix + ".protocol", "must be 'TCP' or 'UDP'"})
	}

	return errors
}

func validateProbe(probe *Probe, probeType string, containerPrefix string) []ValidationError {
	var errors []ValidationError
	probePrefix := containerPrefix + "." + probeType

	// Валидация httpGet
	errors = append(errors, validateHTTPGetAction(&probe.HTTPGet, probePrefix)...)

	return errors
}

func validateHTTPGetAction(action *HTTPGetAction, probePrefix string) []ValidationError {
	var errors []ValidationError

	if action.Path == "" {
		errors = append(errors, ValidationError{probePrefix + ".httpGet.path", "is required"})
	} else if !strings.HasPrefix(action.Path, "/") {
		errors = append(errors, ValidationError{probePrefix + ".httpGet.path", "must be absolute path"})
	}

	if action.Port <= 0 || action.Port >= 65536 {
		errors = append(errors, ValidationError{probePrefix + ".httpGet.port", "must be in range 0 < x < 65536"})
	}

	return errors
}

func validateResourceRequirements(resources *ResourceRequirements, containerPrefix string) []ValidationError {
	var errors []ValidationError
	resourcesPrefix := containerPrefix + ".resources"

	// Валидация requests
	if resources.Requests != nil {
		requestErrors := validateResourceMap(resources.Requests, "requests", resourcesPrefix)
		errors = append(errors, requestErrors...)
	}

	// Валидация limits
	if resources.Limits != nil {
		limitErrors := validateResourceMap(resources.Limits, "limits", resourcesPrefix)
		errors = append(errors, limitErrors...)
	}

	return errors
}

func validateResourceMap(resourceMap map[string]interface{}, mapType string, resourcesPrefix string) []ValidationError {
	var errors []ValidationError
	mapPrefix := resourcesPrefix + "." + mapType

	for resource, value := range resourceMap {
		switch resource {
		case "cpu":
			switch v := value.(type) {
			case int:
				if v <= 0 {
					errors = append(errors, ValidationError{mapPrefix + ".cpu", "must be positive integer"})
				}
			case string:
				if cpu, err := strconv.Atoi(v); err == nil {
					if cpu <= 0 {
						errors = append(errors, ValidationError{mapPrefix + ".cpu", "must be positive integer"})
					}
				} else {
					errors = append(errors, ValidationError{mapPrefix + ".cpu", "must be integer"})
				}
			default:
				errors = append(errors, ValidationError{mapPrefix + ".cpu", "must be integer"})
			}

		case "memory":
			if memory, ok := value.(string); ok {
				if !isValidMemoryValue(memory) {
					errors = append(errors, ValidationError{mapPrefix + ".memory", "must be in format with Gi, Mi, or Ki suffix"})
				}
			} else {
				errors = append(errors, ValidationError{mapPrefix + ".memory", "must be string"})
			}

		default:
			errors = append(errors, ValidationError{mapPrefix + "." + resource, "unknown resource type"})
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
	validationErrors := validatePod(&pod)

	if len(validationErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Validation failed:\n")
		for _, err := range validationErrors {
			fmt.Fprintf(os.Stderr, "  - %s\n", err.Error())
		}
		os.Exit(1)
	}

	fmt.Println("YAML configuration is valid!")
	os.Exit(0)
}