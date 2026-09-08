package mysqltest

import "testing"

func TestLocalEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"npipe:////./pipe/docker_engine",
		"npipe:////./pipe/dockerDesktopLinuxEngine",
		"unix:///var/run/docker.sock",
		"unix:///run/user/1000/docker.sock",
	} {
		t.Run(endpoint, func(t *testing.T) {
			if !localEndpoint(endpoint) {
				t.Fatal("local endpoint rejected")
			}
		})
	}
	for _, endpoint := range []string{
		"npipe:////remote-host/pipe/docker_engine",
		"npipe://remote-host/pipe/docker_engine",
		"npipe:////localhost/pipe/docker_engine",
		"npipe:////./pipe/",
		"npipe:////./pipe/../remote-host/pipe/docker_engine",
		"npipe:////./pipe/%2e%2e%2fremote-host",
		"npipe:////./pipe/docker_engine?host=remote-host",
		`npipe:////./pipe/..\remote-host`,
		"unix://remote-host/run/docker.sock",
		"unix:run/docker.sock",
		"unix://run/docker.sock",
		"unix:////remote-host/run/docker.sock",
		"unix:///run/../run/docker.sock",
		"unix:///run/docker.sock#fragment",
		"unix:///run/docker.sock?",
		"unix:///",
		"tcp://127.0.0.1:2375",
		"tcp://remote-host:2375",
		"ssh://remote-host",
		"",
	} {
		t.Run(endpoint, func(t *testing.T) {
			if localEndpoint(endpoint) {
				t.Fatal("non-local or ambiguous endpoint accepted")
			}
		})
	}
}
