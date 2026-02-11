package main

import "strings"

const adminBasePath = "/backendSalsSavvyLLMRouter"

func adminStaticPath() string {
	return adminBasePath + "/"
}

func adminAPIPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "/" {
		return adminBasePath
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return adminBasePath + trimmed
}
