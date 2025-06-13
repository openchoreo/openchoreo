package request

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type Param struct {
	present bool // Whether available in the context
	Value   string
}

type RequestInfo struct {
	// Hierarchy information about the request
	Params map[ResourceType]Param
}

type ResourceType string // Resource type of the request

const (
	ProjectType     ResourceType = "projects"
	ComponentType   ResourceType = "components"
	DeploymentType  ResourceType = "deployments"
	EnvironmentType ResourceType = "environments"
)

// ParseResourceType parses a string to a resource type with error checking
func ParseResourceType(s string) (ResourceType, error) {
	switch s {
	case string(ProjectType):
		return ProjectType, nil
	case string("components"):
		return ComponentType, nil
	case string("deployments"):
		return DeploymentType, nil
	case string("environments"):
		return EnvironmentType, nil
	default:
		return "", fmt.Errorf("invalid resource type: %s", s)
	}
}

// NewRequestInfo extracts hierarchy information from the request url
// api/v1/projects/<project_id>/components/<component_id>/deployments/<deployment_id>
// api/v1/projects/<project_id>/components/<component_id>
// api/v1/projects/<project_id>/events
func NewRequestInfo(req *http.Request) (*RequestInfo, error) {
	splitPath, err := SplitPath(req.URL.Path)
	if err != nil {
		return nil, err
	}

	params := map[ResourceType]Param{
		ProjectType:     {present: false, Value: ""},
		ComponentType:   {present: false, Value: ""},
		DeploymentType:  {present: false, Value: ""},
		EnvironmentType: {present: false, Value: ""},
	}

	// Parse the hierarchical structure
	for i := 2; i < len(splitPath); i += 2 {
		if i+1 >= len(splitPath) {
			break
		}
		resType, err := ParseResourceType(splitPath[i])
		if err != nil {
			return nil, err
		}
		value := splitPath[i+1]
		params[resType] = Param{
			present: true,
			Value:   value,
		}
	}
	return &RequestInfo{
		Params: params,
	}, nil
}

type RequestInfoKey struct{}

func WithRequestInfo(ctx context.Context, reqInfo *RequestInfo) context.Context {
	return context.WithValue(ctx, RequestInfoKey{}, reqInfo)
}

func SplitPath(p string) ([]string, error) {
	splitPath := strings.Split(strings.Trim(p, "/"), "/")
	if len(splitPath) == 0 || splitPath[0] != "api" {
		return nil, fmt.Errorf("invalid request")
	}
	return splitPath, nil
}
