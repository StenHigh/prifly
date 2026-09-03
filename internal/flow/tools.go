package flow

import "encoding/json"

const ToolDescriptorVersion = "1"

// ParseToolDescriptor accepts the closed protocol shape only. It does not
// resolve dependencies or confer permission to execute the declared tool.
func ParseToolDescriptor(data []byte) (ToolDescriptor, error) {
	value, err := Parse(data, "json")
	if err != nil {
		return ToolDescriptor{}, err
	}
	if err := validateProtocolValue("ToolDescriptor", value, ""); err != nil {
		return ToolDescriptor{}, err
	}
	var descriptor ToolDescriptor
	if err := decodeValue(value, &descriptor); err != nil {
		return ToolDescriptor{}, err
	}
	return descriptor, nil
}

func ValidateToolDescriptor(descriptor ToolDescriptor) error {
	data, err := json.Marshal(descriptor)
	if err != nil {
		return err
	}
	_, err = ParseToolDescriptor(data)
	return err
}
