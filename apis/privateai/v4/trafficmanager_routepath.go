package v4

import (
	"fmt"
	"regexp"
	"strings"
)

var trafficManagerRoutePathPattern = regexp.MustCompile(`^/(?:[A-Za-z0-9._~-]+/)+$`)

func ValidateTrafficManagerRoutePath(routePath string) error {
	routePath = strings.TrimSpace(routePath)
	if routePath == "" {
		return nil
	}
	if !trafficManagerRoutePathPattern.MatchString(routePath) {
		return fmt.Errorf("spec.networking.trafficManager.routePath or deprecated spec.trafficManager.routePath must be an absolute path ending with '/' and contain only letters, numbers, '.', '_', '~', '-' and '/'")
	}
	return nil
}
