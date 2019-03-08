package loader

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"strings"

	//"github.com/docker/stacks/pkg/compose/defaults"
	"github.com/docker/stacks/pkg/compose/interpolation"
	"github.com/docker/stacks/pkg/compose/schema"
	"github.com/docker/stacks/pkg/compose/template"
	composetypes "github.com/docker/stacks/pkg/compose/types"
	"github.com/docker/stacks/pkg/types"
	"github.com/pkg/errors"
)

// TODO - this file needs some refactoring

// LoadComposefile will load the compose files into ComposeInput which can be sent to the server
// for parsing into a Stack representation
func LoadComposefile(composefiles []string) (*types.ComposeInput, error) {
	input := types.ComposeInput{}
	for _, filename := range composefiles {
		bytes, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		input.ComposeFiles = append(input.ComposeFiles, string(bytes))
	}
	return &input, nil
}

// TODO Remainder of this is server side logic that should move someplace else...

// ParseComposeInput will convert the ComposeInput into the StackCreate type
// If the ComposeInput contains any variables, those will be
// listed in the PropertyValues field, so they can be filled
// in prior to sending the StackCreate to the Create API.  If defaults
// are defined in the compose file(s) those defaults will be included.
func ParseComposeInput(input types.ComposeInput) (*types.StackCreate, error) {
	if len(input.ComposeFiles) == 0 {
		return nil, nil
	}

	propertiesMap := map[string]string{}
	for _, tmpl := range input.ComposeFiles {
		matches := template.DefaultPattern.FindAllStringSubmatch(tmpl, -1)
		for _, match := range matches {
			groups := matchGroups(match, template.DefaultPattern)
			if escaped := groups["escaped"]; escaped != "" {
				// Skip escaped ones
				continue
			}
			substitution := groups["named"]
			if substitution == "" {
				substitution = groups["braced"]
			}
			matched := false
			// Check for default values
			var name, defaultValue, errString string
			for _, sep := range []string{":-", "-"} {
				name, defaultValue = partition(substitution, sep)
				if defaultValue != "" {
					propertiesMap[name] = defaultValue
					matched = true
					break
				}
			}
			// Check for mandatory fields
			if !matched {
				for _, sep := range []string{":?", "?"} {
					name, errString = partition(substitution, sep)
					if errString != "" {
						break
					}
					// Don't clobber prior default values if they exist
					if _, exists := propertiesMap[name]; !exists {
						propertiesMap[name] = ""
					}
				}
			}

		}
	}
	properties := []string{}
	for key, value := range propertiesMap {
		if len(value) > 0 {
			properties = append(properties, fmt.Sprintf("%s=%s", key, value))
		} else {
			properties = append(properties, key)
		}
	}

	return &types.StackCreate{
		Templates:      input.ComposeFiles,
		PropertyValues: properties,
	}, nil
}

// ConvertStackCreate will convert the StackCreate type
// into a StackSpec
func ConvertStackCreate(input types.StackCreate) (*types.StackSpec, error) {
	fmt.Printf("XXX in loader.ConvertStackCreate\n")
	if len(input.Templates) == 0 {
		fmt.Printf("XXX no input templates")
		return nil, nil
	}
	fmt.Printf("XXX calling getConfigDetails\n")
	configDetails, err := getConfigDetails(input)
	if err != nil {
		return nil, err
	}

	fmt.Printf("XXX calling getDictsFrom\n")
	dicts := getDictsFrom(configDetails.ConfigFiles)

	// Wire up interpolation as a no-op so we can track the variables in play and default values
	propertiesMap := map[string]string{}
	for _, val := range input.PropertyValues {
		pair := strings.SplitN(val, "=", 2)
		if len(pair) == 2 {
			propertiesMap[pair[0]] = pair[1]
		} else {
			propertiesMap[pair[0]] = ""
		}
	}
	interpolateOpts := interpolation.Options{
		LookupValue: func(key string) (string, bool) {
			val, found := propertiesMap[key]
			fmt.Printf("XXX looking up %s=%s\n", key, val)
			return val, found
		},
		TypeCastMapping: interpolateTypeCastMapping,
		Substitute:      template.Substitute,
	}
	fmt.Printf("XXX performing Load\n")
	config, err := Load(configDetails, func(opts *Options) {
		opts.Interpolate = &interpolateOpts
		//opts.SkipValidation = true
	})
	if err != nil {
		fmt.Printf("XXX something went wrong\n")
		if fpe, ok := err.(*ForbiddenPropertiesError); ok {
			return nil, errors.Errorf("Compose file contains unsupported options:\n\n%s\n",
				propertyWarnings(fpe.Properties))
		}

		return nil, err
	}

	unsupportedProperties := GetUnsupportedProperties(dicts...)
	if len(unsupportedProperties) > 0 {
		fmt.Printf("Ignoring unsupported options: %s\n\n",
			strings.Join(unsupportedProperties, ", "))
	}

	deprecatedProperties := GetDeprecatedProperties(dicts...)
	if len(deprecatedProperties) > 0 {
		fmt.Printf("Ignoring deprecated options:\n\n%s\n\n",
			propertyWarnings(deprecatedProperties))
	}
	properties := []string{}
	for key, value := range propertiesMap {
		if len(value) > 0 {
			properties = append(properties, fmt.Sprintf("%s=%s", key, value))
		} else {
			properties = append(properties, key)
		}
	}
	fmt.Printf("XXX returning valid spec\n")
	return &types.StackSpec{
		Metadata:       input.Metadata,
		Templates:      input.Templates,
		Services:       config.Services,
		Secrets:        config.Secrets,
		Configs:        config.Configs,
		Networks:       config.Networks,
		Volumes:        config.Volumes,
		PropertyValues: properties,
	}, nil
}

// Split the string at the first occurrence of sep, and return the part before the separator,
// and the part after the separator.
//
// If the separator is not found, return the string itself, followed by an empty string.
func partition(s, sep string) (string, string) {
	if strings.Contains(s, sep) {
		parts := strings.SplitN(s, sep, 2)
		return parts[0], parts[1]
	}
	return s, ""
}

func matchGroups(matches []string, pattern *regexp.Regexp) map[string]string {
	groups := make(map[string]string)
	for i, name := range pattern.SubexpNames()[1:] {
		groups[name] = matches[i+1]
	}
	return groups
}

func getDictsFrom(configFiles []composetypes.ConfigFile) []map[string]interface{} {
	dicts := []map[string]interface{}{}

	for _, configFile := range configFiles {
		dicts = append(dicts, configFile.Config)
	}

	return dicts
}

func propertyWarnings(properties map[string]string) string {
	var msgs []string
	for name, description := range properties {
		msgs = append(msgs, fmt.Sprintf("%s: %s", name, description))
	}
	sort.Strings(msgs)
	return strings.Join(msgs, "\n\n")
}

func getConfigDetails(input types.StackCreate) (composetypes.ConfigDetails, error) {
	var details composetypes.ConfigDetails

	var err error
	details.ConfigFiles, err = loadConfigFiles(input)
	if err != nil {
		return details, err
	}
	// Take the first file version (2 files can't have different version)
	details.Version = schema.Version(details.ConfigFiles[0].Config)
	return details, err
}

func loadConfigFiles(input types.StackCreate) ([]composetypes.ConfigFile, error) {
	var configFiles []composetypes.ConfigFile

	for _, data := range input.Templates {
		configFile, err := loadConfigFile(data)
		if err != nil {
			return configFiles, err
		}
		configFiles = append(configFiles, *configFile)
	}

	return configFiles, nil
}

func loadConfigFile(data string) (*composetypes.ConfigFile, error) {
	var err error

	config, err := ParseYAML([]byte(data))
	if err != nil {
		return nil, err
	}

	return &composetypes.ConfigFile{
		Config: config,
	}, nil
}
