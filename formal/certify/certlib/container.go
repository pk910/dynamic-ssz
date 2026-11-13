package certlib

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// containerJSON renders a container or progressive container in the JSON
// declaration spelling, each field's type rendered by render.
func containerJSON(desc *ssztypes.TypeDescriptor, render func(*ssztypes.TypeDescriptor) (string, error)) (string, error) {
	if desc.ContainerDesc == nil {
		return "", fmt.Errorf("container without field descriptors")
	}
	fields := make([]string, 0, len(desc.ContainerDesc.Fields))
	maxIdx := -1
	for i := range desc.ContainerDesc.Fields {
		f := &desc.ContainerDesc.Fields[i]
		e, err := render(f.Type)
		if err != nil {
			return "", fmt.Errorf("field %s: %w", f.Name, err)
		}
		if strings.ContainsAny(f.Name, " \t\r\n\"\\") {
			return "", fmt.Errorf("field name %q cannot be declared inline", f.Name)
		}
		fields = append(fields, `{"name":`+strconv.Quote(f.Name)+`,"type":`+e+`}`)
		if idx := int(f.SszIndex); idx > maxIdx {
			maxIdx = idx
		}
	}
	if desc.SszType == ssztypes.SszProgressiveContainerType {
		layout := make([]string, maxIdx+1)
		for i := range layout {
			layout[i] = "0"
		}
		for i := range desc.ContainerDesc.Fields {
			layout[desc.ContainerDesc.Fields[i].SszIndex] = "1"
		}
		return `{"kind":"ProgressiveContainer","activeFields":[` + strings.Join(layout, ",") +
			`],"fields":[` + strings.Join(fields, ",") + `]}`, nil
	}
	return `{"kind":"Container","fields":[` + strings.Join(fields, ",") + `]}`, nil
}
