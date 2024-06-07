package costmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/prometheus/common/model"
)

func TestNewClient(t *testing.T) {
	type args struct {
		config *ClientConfig
	}
	tests := []struct {
		name  string
		args  args
		want  bool
		error error
	}{
		{
			name: "happy path",
			args: args{
				config: &ClientConfig{
					Address: "http://localhost:9090",
				},
			},
			want:  true,
			error: nil,
		},
		{
			name: "no address",
			args: args{
				config: &ClientConfig{},
			},
			want:  false,
			error: ErrEmptyAddress,
		},
		{
			name: "nil config",
			args: args{
				config: nil,
			},
			want:  false,
			error: ErrNilConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewClient(tt.args.config)
			if err != nil && !errors.Is(err, tt.error) {
				t.Errorf("Unexpected error type error = %v, wantErr %v", err, tt.error)
				return
			}

			if got == nil && tt.want {
				t.Errorf("NewClient() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewClients(t *testing.T) {
	type args struct {
		devConfig  *ClientConfig
		prodConfig *ClientConfig
	}
	tests := []struct {
		name     string
		args     args
		wantDev  bool
		wantProd bool
		error    error
	}{
		{
			name: "happy path",
			args: args{
				devConfig: &ClientConfig{
					Address: "http://localhost:9090",
				},
				prodConfig: &ClientConfig{
					Address: "http://localhost:9090",
				},
			},
			wantDev:  true,
			wantProd: true,
			error:    nil,
		},
		{
			name: "dev is missing",
			args: args{
				devConfig: nil,
				prodConfig: &ClientConfig{
					Address: "http://localhost:9090",
				},
			},
			wantDev:  false,
			wantProd: true,
			error:    nil,
		},
		{
			name: "prod is missing",
			args: args{
				devConfig: &ClientConfig{
					Address: "http://localhost:9090",
				},
				prodConfig: nil,
			},
			wantDev:  false,
			wantProd: false,
			error:    ErrProdConfigMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewClients(tt.args.prodConfig, tt.args.devConfig)
			if err != nil && !errors.Is(err, tt.error) {
				t.Errorf("Unexpected error type error = %v, wantErr %v", err, tt.error)
				return
			}
			if tt.wantDev && got.Dev == nil {
				t.Errorf("NewClient() got = %v, want %v", got.Dev, tt.wantDev)
				return
			}
			if tt.wantProd && got.Prod == nil {
				t.Errorf("NewClient() got = %v, want %v", got.Prod, tt.wantProd)
			}
		})
	}
}

func TestNewClientAuthMethods(t *testing.T) {
	t.Run("basic auth with username and password", func(t *testing.T) {
		cfg := &ClientConfig{
			Address:  "http://localhost:9090",
			Username: "testing",
			Password: "12345",
		}

		client, err := NewClient(cfg)
		if err != nil {
			t.Errorf("NewClient() error = %v, wantErr %v", err, false)
			return
		}
		if client == nil {
			t.Errorf("NewClient() got = %v, want %v", client, true)
		}
	})

	t.Run("basic auth with http config file", func(t *testing.T) {
		tmpCfg, err := os.CreateTemp("", "http_config.yaml")
		if err != nil {
			t.Errorf("error creating temp file: %v", err)
			return
		}
		defer os.Remove(tmpCfg.Name())
		content := fmt.Sprintf("basic_auth:\n  username: %s\n  password: %s", "testing", "12345")
		_, err = tmpCfg.WriteString(content)
		if err != nil {
			t.Errorf("error writing to temp file: %v", err)
			return
		}
		cfg := &ClientConfig{
			Address:        "http://localhost:9090",
			HTTPConfigFile: tmpCfg.Name(),
		}
		client, err := NewClient(cfg)
		if err != nil {
			t.Errorf("NewClient() error = %v, wantErr %v", err, false)
			return
		}
		if client == nil {
			t.Errorf("NewClient() got = %v, want %v", client, true)
		}
	})
}

func TestParseResults(t *testing.T) {
	tests := map[string]struct {
		in  model.Vector
		exp Cost
		err error
	}{
		"empty": {
			model.Vector{},
			Cost{},
			ErrNoResults,
		},

		"memory": {
			model.Vector{
				&model.Sample{Value: 3.14},
			},
			Cost{Dollars: 3.14},
			nil,
		},

		"cpu": {
			model.Vector{
				&model.Sample{Metric: model.Metric{"spot": "false"}, Value: 2.71},
				&model.Sample{Metric: model.Metric{"spot": "true"}, Value: 1.41},
			},
			Cost{Spot: 1.41, NonSpot: 2.71},
			nil,
		},
	}

	c := &Client{}

	for n, tt := range tests {
		t.Run(n, func(t *testing.T) {
			got, err := c.parseResults(tt.in)
			if !errors.Is(tt.err, err) {
				t.Fatalf("expecting error %v, got %v", tt.err, err)
			}

			if got != tt.exp {
				t.Fatalf("expecting cost %v, got %v", tt.exp, got)
			}
		})
	}
}

func TestClient_GetNodeCount(t *testing.T) {
	type Result struct {
		Metric model.Metric     `json:"metric"`
		Value  model.SamplePair `json:"value"`
	}
	type mockQueryRangeResponse struct {
		Status string `json:"status"`
		Data   struct {
			Type   string   `json:"resultType"`
			Result []Result `json:"result"`
		} `json:"data"`
	}

	type args struct {
		ctx     context.Context
		cluster string
	}
	tests := []struct {
		name     string
		args     args
		response *mockQueryRangeResponse
		want     int
		wantErr  error
	}{
		{
			"Respones with a single value.",
			args{
				context.Background(),
				"test",
			},
			&mockQueryRangeResponse{
				Status: "success",
				Data: struct {
					Type   string   `json:"resultType"`
					Result []Result `json:"result"`
				}{
					Type: "vector",
					Result: []Result{
						{
							Metric: model.Metric{},
							Value: model.SamplePair{
								Timestamp: model.TimeFromUnix(0),
								Value:     1,
							},
						},
					},
				},
			},
			1,
			nil,
		},
		{
			"Prometheus responds with multiple values and GetNodeCount returns first value",
			args{
				context.Background(),
				"test",
			},
			&mockQueryRangeResponse{
				Status: "success",
				Data: struct {
					Type   string   `json:"resultType"`
					Result []Result `json:"result"`
				}{
					Type: "vector",
					Result: []Result{
						{
							Metric: model.Metric{},
							Value: model.SamplePair{
								Timestamp: model.TimeFromUnix(0),
								Value:     10,
							},
						},
						{
							Metric: model.Metric{},
							Value: model.SamplePair{
								Timestamp: model.TimeFromUnix(0),
								Value:     100,
							},
						},
					},
				},
			},
			10,
			nil,
		},
		{
			"Responds with an error if results is nil.",
			args{
				context.Background(),
				"test",
			},
			&mockQueryRangeResponse{
				Status: "success",
				Data: struct {
					Type   string   `json:"resultType"`
					Result []Result `json:"result"`
				}{
					Type: "vector",
				},
			},
			0,
			ErrBadQuery,
		},
		{
			"Responds with an error if there are no values.",
			args{
				context.Background(),
				"test",
			},
			&mockQueryRangeResponse{
				Status: "success",
				Data: struct {
					Type   string   `json:"resultType"`
					Result []Result `json:"result"`
				}{
					Type:   "vector",
					Result: []Result{},
				},
			},
			0,
			ErrNoResults,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := tt.response
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Errorf("error encoding response: %v", err)
					return
				}
			}))

			defer svr.Close()
			c, err := NewClient(&ClientConfig{
				Address: svr.URL,
			})
			if err != nil {
				t.Errorf("error creating client: %v", err)
				return
			}
			got, err := c.GetNodeCount(tt.args.ctx, tt.args.cluster)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetNodeCount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Client.GetNodeCount() = %v, want %v", got, tt.want)
			}
		})
	}
}
