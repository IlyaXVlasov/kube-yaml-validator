package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Структуры для данных
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
	OS        *PodOS      `yaml:"os,omitempty"`
	Containers []Container `yaml:"containers"`
}

type PodOS struct {
	Name string `yaml:"name"`
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

// Валидация с использованием yaml.Node
func validateYAML(data []byte, filename string) []string {
	var errors []string

	// Парсим в yaml.Node для получения информации о строках
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return []string{fmt.Sprintf("Error parsing YAML: %v", err)}
	}

	// Парсим в структуру Pod для валидации
	var pod Pod
	if err := yaml.Unmarshal(data, &pod); err != nil {
		return []string{fmt.Sprintf("Error parsing YAML: %v", err)}
	}

	// Валидируем поля верхнего уровня
	if pod.APIVersion != "v1" {
		line := findLine(&root, "apiVersion")
		errors = append(errors, fmt.Sprintf("%s:%d apiVersion must be 'v1'", filename, line))
	}

	if pod.Kind != "Pod" {
		line := findLine(&root, "kind")
		errors = append(errors, fmt.Sprintf("%s:%d kind must be 'Pod'", filename, line))
	}

	// Валидация metadata
	if pod.Metadata.Name == "" {
		line := findLine(&root, "name")
		errors = append(errors, fmt.Sprintf("%s:%d name is required", filename, line))
	}

	// Валидация spec.os
	if pod.Spec.OS != nil {
		if pod.Spec.OS.Name != "linux" && pod.Spec.OS.Name != "windows" {
			line := findLine(&root, "os")
			errors = append(errors, fmt.Sprintf("%s:%d os has unsupported value '%s'", filename, line, pod.Spec.OS.Name))
		}
	}

	// Валидация containers
	if len(pod.Spec.Containers) == 0 {
		line := findLine(&root, "containers")
		errors = append(errors, fmt.Sprintf("%s:%d at least one container is required", filename, line))
	} else {
		for i, container := range pod.Spec.Containers {
			containerErrors := validateContainer(container, i, &root, filename)
			errors = append(errors, containerErrors...)
		}
	}

	return errors
}

func validateContainer(container Container, index int, root *yaml.Node, filename string) []string {
	var errors []string

	// Валидация имени контейнера
	if container.Name == "" {
		line := findNestedLine(root, "containers", index, "name")
		errors = append(errors, fmt.Sprintf("%s:%d container name is required", filename, line))
	} else {
		snakeCaseRegex := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
		if !snakeCaseRegex.MatchString(container.Name) {
			line := findNestedLine(root, "containers", index, "name")
			errors = append(errors, fmt.Sprintf("%s:%d container name must be in snake_case format", filename, line))
		}
	}

	// Валидация image
	if container.Image == "" {
		line := findNestedLine(root, "containers", index, "image")
		errors = append(errors, fmt.Sprintf("%s:%d image is required", filename, line))
	} else {
		if !strings.HasPrefix(container.Image, "registry.bigbrother.io/") {
			line := findNestedLine(root, "containers", index, "image")
			errors = append(errors, fmt.Sprintf("%s:%d image must be in domain registry.bigbrother.io", filename, line))
		}
		
		parts := strings.Split(container.Image, ":")
		if len(parts) != 2 || parts[1] == "" {
			line := findNestedLine(root, "containers", index, "image")
			errors = append(errors, fmt.Sprintf("%s:%d image must have version tag", filename, line))
		}
	}

	// Валидация ports
	for _, port := range container.Ports {
		if port.ContainerPort <= 0 || port.ContainerPort >= 65536 {
			line := findNestedLine(root, "containers", index, "containerPort")
			errors = append(errors, fmt.Sprintf("%s:%d containerPort value out of range", filename, line))
		}

		if port.Protocol != "" && port.Protocol != "TCP" && port.Protocol != "UDP" {
			line := findNestedLine(root, "containers", index, "protocol")
			errors = append(errors, fmt.Sprintf("%s:%d protocol must be 'TCP' or 'UDP'", filename, line))
		}
	}

	// Валидация readinessProbe
	if container.ReadinessProbe != nil {
		if container.ReadinessProbe.HTTPGet.Port <= 0 || container.ReadinessProbe.HTTPGet.Port >= 65536 {
			line := findNestedLine(root, "containers", index, "port")
			errors = append(errors, fmt.Sprintf("%s:%d port value out of range", filename, line))
		}

		if container.ReadinessProbe.HTTPGet.Path == "" {
			line := findNestedLine(root, "containers", index, "path")
			errors = append(errors, fmt.Sprintf("%s:%d path is required", filename, line))
		} else if !strings.HasPrefix(container.ReadinessProbe.HTTPGet.Path, "/") {
			line := findNestedLine(root, "containers", index, "path")
			errors = append(errors, fmt.Sprintf("%s:%d path must be absolute", filename, line))
		}
	}

	// Валидация livenessProbe
	if container.LivenessProbe != nil {
		if container.LivenessProbe.HTTPGet.Port <= 0 || container.LivenessProbe.HTTPGet.Port >= 65536 {
			line := findNestedLine(root, "containers", index, "port")
			errors = append(errors, fmt.Sprintf("%s:%d port value out of range", filename, line))
		}

		if container.LivenessProbe.HTTPGet.Path == "" {
			line := findNestedLine(root, "containers", index, "path")
			errors = append(errors, fmt.Sprintf("%s:%d path is required", filename, line))
		} else if !strings.HasPrefix(container.LivenessProbe.HTTPGet.Path, "/") {
			line := findNestedLine(root, "containers", index, "path")
			errors = append(errors, fmt.Sprintf("%s:%d path must be absolute", filename, line))
		}
	}

	// Валидация resources - УПРОЩЕННАЯ ВЕРСИЯ
	if container.Resources.Limits != nil {
		if cpu, ok := container.Resources.Limits["cpu"]; ok {
			switch v := cpu.(type) {
			case int:
				if v <= 0 {
					// Используем приблизительный номер строки для resources
					line := findNestedLine(root, "containers", index, "resources") + 2
					errors = append(errors, fmt.Sprintf("%s:%d cpu must be positive integer", filename, line))
				}
			case string:
				if _, err := strconv.Atoi(v); err != nil {
					line := findNestedLine(root, "containers", index, "resources") + 2
					errors = append(errors, fmt.Sprintf("%s:%d cpu must be int", filename, line))
				}
			default:
				line := findNestedLine(root, "containers", index, "resources") + 2
				errors = append(errors, fmt.Sprintf("%s:%d cpu must be integer", filename, line))
			}
		}
	}

	if container.Resources.Requests != nil {
		if cpu, ok := container.Resources.Requests["cpu"]; ok {
			switch v := cpu.(type) {
			case int:
				if v <= 0 {
					line := findNestedLine(root, "containers", index, "resources") + 5
					errors = append(errors, fmt.Sprintf("%s:%d cpu must be positive integer", filename, line))
				}
			case string:
				if _, err := strconv.Atoi(v); err != nil {
					line := findNestedLine(root, "containers", index, "resources") + 5
					errors = append(errors, fmt.Sprintf("%s:%d cpu must be int", filename, line))
				}
			default:
				line := findNestedLine(root, "containers", index, "resources") + 5
				errors = append(errors, fmt.Sprintf("%s:%d cpu must be integer", filename, line))
			}
		}
	}

	return errors
}

// Вспомогательные функции для поиска номеров строк
func findLine(root *yaml.Node, field string) int {
	for _, doc := range root.Content {
		for i := 0; i < len(doc.Content); i += 2 {
			if i < len(doc.Content) && doc.Content[i].Value == field {
				return doc.Content[i].Line
			}
		}
	}
	return 1
}

func findNestedLine(root *yaml.Node, parentField string, index int, field string) int {
	for _, doc := range root.Content {
		for i := 0; i < len(doc.Content); i += 2 {
			if i < len(doc.Content) && doc.Content[i].Value == parentField {
				// Находим containers
				containersNode := doc.Content[i+1]
				if containersNode.Kind == yaml.SequenceNode && len(containersNode.Content) > index {
					// Находим конкретный контейнер
					containerNode := containersNode.Content[index]
					for j := 0; j < len(containerNode.Content); j += 2 {
						if j < len(containerNode.Content) && containerNode.Content[j].Value == field {
							return containerNode.Content[j].Line
						}
					}
				}
			}
		}
	}
	return 1
}

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

	// Валидация
	errors := validateYAML(data, filename)

	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}

	fmt.Println("YAML configuration is valid!")
	os.Exit(0)
}