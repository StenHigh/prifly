package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	MaxDocumentBytes = 2 << 20
	MaxDepth         = 64
	MaxNodes         = 100000
	maxSafeInteger   = 9007199254740991
)

// Parse reads one bounded JSON value. YAML is only a representation of that
// same value: aliases, anchors, explicit tags, merge keys and non-JSON scalars
// are rejected before conversion. It does not interpret environment variables.
func Parse(data []byte, format string) (any, error) {
	if len(data) > MaxDocumentBytes {
		return nil, problem("document_too_large", "", "document exceeds 2 MiB")
	}
	if !utf8.Valid(data) {
		return nil, problem("invalid_unicode", "", "input must be valid UTF-8")
	}
	switch format {
	case "json", "":
		return parseJSON(data)
	case "yaml", "yml":
		return parseYAML(data)
	default:
		return nil, problem("unsupported_format", "", "expected json or yaml")
	}
}

// JSONBytes converts the validated JSON data model to machine JSON. It is not
// a claim that encoding/json implements RFC 8785 canonicalization.
func JSONBytes(data []byte, format string) ([]byte, error) {
	value, err := Parse(data, format)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func parseJSON(data []byte) (any, error) {
	if err := checkSurrogates(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	nodes := 0
	value, err := jsonValue(decoder, 0, "", &nodes)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, problem("invalid_json", "", "expected exactly one JSON value")
	}
	return value, nil
}

func jsonValue(decoder *json.Decoder, depth int, path string, nodes *int) (any, error) {
	*nodes++
	if depth > MaxDepth || *nodes > MaxNodes {
		return nil, problem("document_limit", path, "maximum depth or node count exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, problem("invalid_json", path, "invalid JSON syntax")
	}
	switch token := token.(type) {
	case json.Delim:
		switch token {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return nil, problem("invalid_json", path, "invalid JSON syntax")
				}
				name, ok := key.(string)
				if !ok {
					return nil, problem("invalid_json", path, "object key must be a string")
				}
				child := path + "/" + escapePointer(name)
				if _, exists := object[name]; exists {
					return nil, problem("duplicate_key", child, "duplicate object key")
				}
				value, err := jsonValue(decoder, depth+1, child, nodes)
				if err != nil {
					return nil, err
				}
				object[name] = value
			}
			if _, err := decoder.Token(); err != nil {
				return nil, problem("invalid_json", path, "invalid JSON syntax")
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for decoder.More() {
				value, err := jsonValue(decoder, depth+1, fmt.Sprintf("%s/%d", path, len(array)), nodes)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, problem("invalid_json", path, "invalid JSON syntax")
			}
			return array, nil
		default:
			return nil, problem("invalid_json", path, "unexpected closing delimiter")
		}
	case json.Number:
		return number(string(token), path)
	default:
		return token, nil
	}
}

var jsonNumber = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

func number(raw, path string) (json.Number, error) {
	if !jsonNumber.MatchString(raw) {
		return "", problem("invalid_number", path, "number must use JSON decimal syntax")
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return "", problem("invalid_number", path, "number is outside finite IEEE-754 range")
	}
	if math.Trunc(n) == n && math.Abs(n) > maxSafeInteger {
		return "", problem("unsafe_integer", path, "exact large integers must be encoded as strings")
	}
	return json.Number(raw), nil
}

func parseYAML(data []byte) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, problem("invalid_yaml", "", "invalid YAML syntax")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, problem("invalid_yaml", "", "expected exactly one YAML document")
	}
	if len(document.Content) != 1 {
		return nil, problem("invalid_yaml", "", "expected one document value")
	}
	nodes := 0
	return yamlValue(document.Content[0], 0, "", &nodes)
}

func yamlValue(node *yaml.Node, depth int, path string, nodes *int) (any, error) {
	*nodes++
	if depth > MaxDepth || *nodes > MaxNodes {
		return nil, problem("document_limit", path, "maximum depth or node count exceeded")
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 {
		return nil, problem("unsupported_yaml", path, "aliases, anchors and explicit tags are forbidden")
	}
	switch node.Kind {
	case yaml.MappingNode:
		object := make(map[string]any)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" {
				return nil, problem("unsupported_yaml", path, "object keys must be strings; merge keys are forbidden")
			}
			if key.Anchor != "" || key.Style&yaml.TaggedStyle != 0 {
				return nil, problem("unsupported_yaml", path, "tagged or anchored keys are forbidden")
			}
			child := path + "/" + escapePointer(key.Value)
			if _, exists := object[key.Value]; exists {
				return nil, problem("duplicate_key", child, "duplicate object key")
			}
			value, err := yamlValue(node.Content[i+1], depth+1, child, nodes)
			if err != nil {
				return nil, err
			}
			object[key.Value] = value
		}
		return object, nil
	case yaml.SequenceNode:
		array := make([]any, 0, len(node.Content))
		for i, child := range node.Content {
			value, err := yamlValue(child, depth+1, fmt.Sprintf("%s/%d", path, i), nodes)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		return array, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str":
			if node.Style == 0 {
				switch strings.ToLower(node.Value) {
				case "yes", "no", "on", "off", "y", "n":
					return nil, problem("ambiguous_yaml_scalar", path, "quote strings resembling YAML 1.1 booleans")
				}
			}
			return node.Value, nil
		case "!!bool":
			if node.Value != "true" && node.Value != "false" {
				return nil, problem("ambiguous_yaml_scalar", path, "use lowercase true or false")
			}
			return node.Value == "true", nil
		case "!!null":
			if node.Value != "null" {
				return nil, problem("ambiguous_yaml_scalar", path, "use explicit lowercase null")
			}
			return nil, nil
		case "!!int", "!!float":
			return number(node.Value, path)
		default:
			return nil, problem("unsupported_yaml", path, "only JSON scalar types are supported; quote timestamps")
		}
	default:
		return nil, problem("unsupported_yaml", path, "expected a JSON-compatible value")
	}
}

// encoding/json replaces lone surrogate escapes with U+FFFD. Reject them
// instead, so malformed names cannot silently acquire a different identity.
func checkSurrogates(data []byte) error {
	inside := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inside = !inside
			continue
		}
		if !inside || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) || data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return problem("invalid_unicode", "", "incomplete Unicode escape")
		}
		code, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return problem("invalid_unicode", "", "invalid Unicode escape")
		}
		i += 4
		if code >= 0xDC00 && code <= 0xDFFF {
			return problem("invalid_unicode", "", "unpaired low surrogate")
		}
		if code < 0xD800 || code > 0xDBFF {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return problem("invalid_unicode", "", "unpaired high surrogate")
		}
		low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
		if err != nil || low < 0xDC00 || low > 0xDFFF {
			return problem("invalid_unicode", "", "invalid surrogate pair")
		}
		i += 6
	}
	return nil
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}
