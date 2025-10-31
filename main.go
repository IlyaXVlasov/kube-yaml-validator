package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s <yaml-file>\n", os.Args[0])
		os.Exit(1)
	}

	filename := os.Args[1]

	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	errors := validateYAML(&root, filename)
	
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("%s\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("YAML configuration is valid!\n")
	os.Exit(0)
}

func validateYAML(root *yaml.Node, filename string) []string {
	var errors []string
	
	for _, doc := range root.Content {
		if doc.Kind == yaml.MappingNode {
			errors = append(errors, validatePod(doc, filename)...)
		}
	}
	
	return errors
}

func validatePod(podNode *yaml.Node, filename string) []string {
	var errors []string
	
	// 1. Поля верхнего уровня - ОБЯЗАТЕЛЬНЫ
	apiVersionNode := findField(podNode, "apiVersion")
	if apiVersionNode == nil {
		errors = append(errors, fmt.Sprintf("%s apiVersion is required", filename))
	} else if apiVersionNode.Value != "v1" {
		errors = append(errors, fmt.Sprintf("%s:%d apiVersion must be 'v1'", filename, apiVersionNode.Line))
	}
	
	kindNode := findField(podNode, "kind")
	if kindNode == nil {
		errors = append(errors, fmt.Sprintf("%s kind is required", filename))
	} else if kindNode.Value != "Pod" {
		errors = append(errors, fmt.Sprintf("%s:%d kind must be 'Pod'", filename, kindNode.Line))
	}
	
	metadataNode := findField(podNode, "metadata")
	if metadataNode == nil {
		errors = append(errors, fmt.Sprintf("%s metadata is required", filename))
	} else {
		errors = append(errors, validateMetadata(metadataNode, filename)...)
	}
	
	specNode := findField(podNode, "spec")
	if specNode == nil {
		errors = append(errors, fmt.Sprintf("%s spec is required", filename))
	} else {
		errors = append(errors, validateSpec(specNode, filename)...)
	}
	
	return errors
}

func validateMetadata(metadataNode *yaml.Node, filename string) []string {
	var errors []string
	
	// 2. ObjectMeta name - ОБЯЗАТЕЛЬНО
	nameNode := findField(metadataNode, "name")
	if nameNode == nil {
		errors = append(errors, fmt.Sprintf("%s name is required", filename))
	} else if nameNode.Kind == yaml.ScalarNode {
		if strings.TrimSpace(nameNode.Value) == "" {
			errors = append(errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
		}
	} else if nameNode.Kind != yaml.ScalarNode {
		errors = append(errors, fmt.Sprintf("%s:%d name must be string", filename, nameNode.Line))
	}
	
	return errors
}

func validateSpec(specNode *yaml.Node, filename string) []string {
	var errors []string
	
	// 4. PodOS (если указан) - должен быть объектом с полем name
	osNode := findField(specNode, "os")
	if osNode != nil {
		if osNode.Kind == yaml.MappingNode {
			osNameNode := findField(osNode, "name")
			if osNameNode == nil {
				errors = append(errors, fmt.Sprintf("%s:%d os name is required", filename, osNode.Line))
			} else if osNameNode.Kind == yaml.ScalarNode {
				if osNameNode.Value != "linux" && osNameNode.Value != "windows" {
					errors = append(errors, fmt.Sprintf("%s:%d os has unsupported value '%s'", filename, osNameNode.Line, osNameNode.Value))
				}
			} else {
				errors = append(errors, fmt.Sprintf("%s:%d os name must be string", filename, osNameNode.Line))
			}
		} else {
			errors = append(errors, fmt.Sprintf("%s:%d os must be object", filename, osNode.Line))
		}
	}
	
	// 3. PodSpec containers - ОБЯЗАТЕЛЬНО
	containersNode := findField(specNode, "containers")
	if containersNode == nil {
		errors = append(errors, fmt.Sprintf("%s containers is required", filename))
	} else if containersNode.Kind == yaml.SequenceNode {
		if len(containersNode.Content) == 0 {
			errors = append(errors, fmt.Sprintf("%s:%d at least one container is required", filename, containersNode.Line))
		} else {
			// Проверка уникальности имен контейнеров
			containerNames := make(map[string]bool)
			for i, containerNode := range containersNode.Content {
				containerErrors := validateContainer(containerNode, i, filename)
				errors = append(errors, containerErrors...)
				
				// Проверка уникальности имени
				nameNode := findField(containerNode, "name")
				if nameNode != nil && nameNode.Kind == yaml.ScalarNode && nameNode.Value != "" {
					if containerNames[nameNode.Value] {
						errors = append(errors, fmt.Sprintf("%s:%d container name must be unique", filename, nameNode.Line))
					}
					containerNames[nameNode.Value] = true
				}
			}
		}
	} else {
		errors = append(errors, fmt.Sprintf("%s:%d containers must be array", filename, containersNode.Line))
	}
	
	return errors
}

func validateContainer(containerNode *yaml.Node, index int, filename string) []string {
	var errors []string
	
	// 5. Container name - ОБЯЗАТЕЛЬНО
	nameNode := findField(containerNode, "name")
	if nameNode == nil {
		errors = append(errors, fmt.Sprintf("%s:%d container name is required", filename, findFieldLine(containerNode, "name")))
	} else if nameNode.Kind == yaml.ScalarNode && nameNode.Value == "" {
		errors = append(errors, fmt.Sprintf("%s:%d name is required", filename, nameNode.Line))
	} else if nameNode.Kind != yaml.ScalarNode {
		errors = append(errors, fmt.Sprintf("%s:%d container name must be string", filename, nameNode.Line))
	} else {
		snakeCaseRegex := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
		if !snakeCaseRegex.MatchString(nameNode.Value) {
			errors = append(errors, fmt.Sprintf("%s:%d container name has invalid format '%s'", filename, nameNode.Line, nameNode.Value))
		}
	}
	
	// 5. Container image - ОБЯЗАТЕЛЬНО
	imageNode := findField(containerNode, "image")
	if imageNode == nil {
		errors = append(errors, fmt.Sprintf("%s:%d image is required", filename, findFieldLine(containerNode, "image")))
	} else if imageNode.Kind == yaml.ScalarNode && imageNode.Value == "" {
		errors = append(errors, fmt.Sprintf("%s:%d image is required", filename, imageNode.Line))
	} else if imageNode.Kind != yaml.ScalarNode {
		errors = append(errors, fmt.Sprintf("%s:%d image must be string", filename, imageNode.Line))
	} else {
		if !strings.HasPrefix(imageNode.Value, "registry.bigbrother.io/") {
			errors = append(errors, fmt.Sprintf("%s:%d image has invalid format '%s'", filename, imageNode.Line, imageNode.Value))
		}
		
		parts := strings.Split(imageNode.Value, ":")
		if len(parts) != 2 || parts[1] == "" {
			errors = append(errors, fmt.Sprintf("%s:%d image has invalid format '%s'", filename, imageNode.Line, imageNode.Value))
		}
	}
	
	// 6. ContainerPort (если указан) containerPort - ОБЯЗАТЕЛЬНО
	portsNode := findField(containerNode, "ports")
	if portsNode != nil {
		if portsNode.Kind == yaml.SequenceNode {
			for _, portNode := range portsNode.Content {
				errors = append(errors, validateContainerPort(portNode, filename)...)
			}
		} else {
			errors = append(errors, fmt.Sprintf("%s:%d ports must be array", filename, portsNode.Line))
		}
	}
	
	// 7. Probe (если указан) httpGet - ОБЯЗАТЕЛЬНО
	readinessProbeNode := findField(containerNode, "readinessProbe")
	if readinessProbeNode != nil {
		errors = append(errors, validateProbe(readinessProbeNode, "readinessProbe", filename)...)
	}
	
	// 7. Probe (если указан) httpGet - ОБЯЗАТЕЛЬНО
	livenessProbeNode := findField(containerNode, "livenessProbe")
	if livenessProbeNode != nil {
		errors = append(errors, validateProbe(livenessProbeNode, "livenessProbe", filename)...)
	}
	
	// 5. Container resources - ОБЯЗАТЕЛЬНО
	resourcesNode := findField(containerNode, "resources")
	if resourcesNode == nil {
		errors = append(errors, fmt.Sprintf("%s:%d resources are required", filename, findFieldLine(containerNode, "resources")))
	} else {
		errors = append(errors, validateResourceRequirements(resourcesNode, filename)...)
	}
	
	return errors
}

func validateContainerPort(portNode *yaml.Node, filename string) []string {
	var errors []string
	
	// 6. ContainerPort containerPort - ОБЯЗАТЕЛЬНО
	containerPortNode := findField(portNode, "containerPort")
	if containerPortNode == nil {
		errors = append(errors, fmt.Sprintf("%s:%d containerPort is required", filename, findFieldLine(portNode, "containerPort")))
	} else if containerPortNode.Kind != yaml.ScalarNode {
		errors = append(errors, fmt.Sprintf("%s:%d containerPort must be int", filename, containerPortNode.Line))
	} else {
		if port, err := strconv.Atoi(containerPortNode.Value); err != nil {
			errors = append(errors, fmt.Sprintf("%s:%d containerPort must be int", filename, containerPortNode.Line))
		} else if port <= 0 || port >= 65536 {
			errors = append(errors, fmt.Sprintf("%s:%d containerPort value out of range", filename, containerPortNode.Line))
		}
	}
	
	// protocol - НЕОБЯЗАТЕЛЬНО
	protocolNode := findField(portNode, "protocol")
	if protocolNode != nil && protocolNode.Kind == yaml.ScalarNode && protocolNode.Value != "" {
		if protocolNode.Value != "TCP" && protocolNode.Value != "UDP" {
			errors = append(errors, fmt.Sprintf("%s:%d protocol has unsupported value '%s'", filename, protocolNode.Line, protocolNode.Value))
		}
	}
	
	return errors
}

func validateProbe(probeNode *yaml.Node, probeType string, filename string) []string {
	var errors []string
	
	// 7. Probe httpGet - ОБЯЗАТЕЛЬНО
	httpGetNode := findField(probeNode, "httpGet")
	if httpGetNode == nil {
		errors = append(errors, fmt.Sprintf("%s:%d httpGet is required", filename, findFieldLine(probeNode, "httpGet")))
	} else {
		errors = append(errors, validateHTTPGetAction(httpGetNode, filename)...)
	}
	
	return errors
}

func validateHTTPGetAction(httpGetNode *yaml.Node, filename string) []string {
	var errors []string
	
	// 8. HTTPGetAction path - ОБЯЗАТЕЛЬНО
	pathNode := findField(httpGetNode, "path")
	if pathNode == nil {
		errors = append(errors, fmt.Sprintf("%s:%d path is required", filename, findFieldLine(httpGetNode, "path")))
	} else if pathNode.Kind == yaml.ScalarNode && pathNode.Value == "" {
		errors = append(errors, fmt.Sprintf("%s:%d path is required", filename, pathNode.Line))
	} else if pathNode.Kind != yaml.ScalarNode {
		errors = append(errors, fmt.Sprintf("%s:%d path must be string", filename, pathNode.Line))
	} else if !strings.HasPrefix(pathNode.Value, "/") {
		errors = append(errors, fmt.Sprintf("%s:%d path has invalid format '%s'", filename, pathNode.Line, pathNode.Value))
	}
	
	// 8. HTTPGetAction port - ОБЯЗАТЕЛЬНО
	portNode := findField(httpGetNode, "port")
	if portNode == nil {
		errors = append(errors, fmt.Sprintf("%s:%d port is required", filename, findFieldLine(httpGetNode, "port")))
	} else if portNode.Kind != yaml.ScalarNode {
		errors = append(errors, fmt.Sprintf("%s:%d port must be int", filename, portNode.Line))
	} else {
		if port, err := strconv.Atoi(portNode.Value); err != nil {
			errors = append(errors, fmt.Sprintf("%s:%d port must be int", filename, portNode.Line))
		} else if port <= 0 || port >= 65536 {
			errors = append(errors, fmt.Sprintf("%s:%d port value out of range", filename, portNode.Line))
		}
	}
	
	return errors
}

func validateResourceRequirements(resourcesNode *yaml.Node, filename string) []string {
	var errors []string
	
	// resources.requests - НЕОБЯЗАТЕЛЬНО
	requestsNode := findField(resourcesNode, "requests")
	if requestsNode != nil {
		errors = append(errors, validateResourceMap(requestsNode, "requests", filename)...)
	}
	
	// resources.limits - НЕОБЯЗАТЕЛЬНО
	limitsNode := findField(resourcesNode, "limits")
	if limitsNode != nil {
		errors = append(errors, validateResourceMap(limitsNode, "limits", filename)...)
	}
	
	return errors
}

func validateResourceMap(resourceMapNode *yaml.Node, mapType string, filename string) []string {
	var errors []string
	
	for i := 0; i < len(resourceMapNode.Content); i += 2 {
		if i < len(resourceMapNode.Content) {
			resourceNameNode := resourceMapNode.Content[i]
			resourceValueNode := resourceMapNode.Content[i+1]
			
			switch resourceNameNode.Value {
			case "cpu":
				if resourceValueNode.Tag != "!!int" {
					errors = append(errors, fmt.Sprintf("%s:%d cpu must be int", filename, resourceValueNode.Line))
				} else if cpu, err := strconv.Atoi(resourceValueNode.Value); err != nil || cpu <= 0 {
					errors = append(errors, fmt.Sprintf("%s:%d cpu must be positive integer", filename, resourceValueNode.Line))
				}
				
			case "memory":
				if resourceValueNode.Tag != "!!str" {
					errors = append(errors, fmt.Sprintf("%s:%d memory must be string", filename, resourceValueNode.Line))
				} else if !isValidMemoryValue(resourceValueNode.Value) {
					errors = append(errors, fmt.Sprintf("%s:%d memory has invalid format '%s'", filename, resourceValueNode.Line, resourceValueNode.Value))
				}
				
			default:
				errors = append(errors, fmt.Sprintf("%s:%d unknown resource type", filename, resourceNameNode.Line))
			}
		}
	}
	
	return errors
}

func isValidMemoryValue(memory string) bool {
	memoryRegex := regexp.MustCompile(`^\d+(Gi|Mi|Ki)$`)
	return memoryRegex.MatchString(memory)
}

func findField(node *yaml.Node, field string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	
	for i := 0; i < len(node.Content); i += 2 {
		if i < len(node.Content) && node.Content[i].Value == field {
			return node.Content[i+1]
		}
	}
	return nil
}

func findFieldLine(node *yaml.Node, field string) int {
	if node == nil || node.Kind != yaml.MappingNode {
		return 1
	}
	
	for i := 0; i < len(node.Content); i += 2 {
		if i < len(node.Content) && node.Content[i].Value == field {
			return node.Content[i].Line
		}
	}
	return 1
}