package extkafka

import (
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_resolveStatusCodeExpression(t *testing.T) {
	type args struct {
		statusCodes string
	}
	tests := []struct {
		name  string
		args  args
		want  []int
		error *action_kit_api.ActionKitError
	}{
		{
			name: "Should return status codes with range",
			args: args{
				statusCodes: "200-209",
			},
			want:  []int{200, 201, 202, 203, 204, 205, 206, 207, 208, 209},
			error: nil,
		},
		{
			name: "Should return status codes with range and enum",
			args: args{
				statusCodes: "201-202;209",
			},
			want:  []int{201, 202, 209},
			error: nil,
		},
		{
			name: "Should return error if invalid status code",
			args: args{
				statusCodes: "600",
			},
			want:  nil,
			error: &action_kit_api.ActionKitError{Title: "Invalid status code '600'. Status code should be between 100 and 599."},
		},
		{
			name: "Should return error if invalid status code range",
			args: args{
				statusCodes: "200-",
			},
			want:  nil,
			error: &action_kit_api.ActionKitError{Title: "Invalid status code range '200-'. Please use '-' for ranges and ';' for enumerations. Example: '200-399;429'"},
		},
		{
			name: "Should return error if invalid status code range",
			args: args{
				statusCodes: "200-;209",
			},
			want:  nil,
			error: &action_kit_api.ActionKitError{Title: "Invalid status code range '200-'. Please use '-' for ranges and ';' for enumerations. Example: '200-399;429'"},
		},
		{
			name: "Should return error if invalid status code range",
			args: args{
				statusCodes: "200-209;600",
			},
			want:  nil,
			error: &action_kit_api.ActionKitError{Title: "Invalid status code '600'. Status code should be between 100 and 599."},
		},
		{
			name: "Should return error if status code is empty",
			args: args{
				statusCodes: "",
			},
			want:  nil,
			error: &action_kit_api.ActionKitError{Title: "Status code is required."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := resolveStatusCodeExpression(tt.args.statusCodes)
			assert.Equalf(t, tt.want, got, "resolveStatusCodeExpression(%v)", tt.args.statusCodes)
			assert.Equalf(t, tt.error, got1, "resolveStatusCodeExpression(%v)", tt.args.statusCodes)
		})
	}
}
