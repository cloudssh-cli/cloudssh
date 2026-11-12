package sshx

import (
	"context"
	"testing"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

const psSample = `/sbin/launchd
/usr/local/bin/aws ec2-instance-connect open-tunnel --instance-id i-0abc --local-port 2222
python3 /usr/local/bin/aws ec2-instance-connect open-tunnel --instance-id=i-0def --local-port=2223 --remote-port 22 --profile prod
aws ec2-instance-connect open-tunnel --private-ip-address 10.0.1.9 --local-port 2224
aws ec2-instance-connect open-tunnel --instance-id i-0ghi --local-port 3306 --remote-port 3306
aws ec2-instance-connect open-tunnel --instance-id i-0xyz --remote-port 22
aws ec2-instance-connect ssh --instance-id i-0abc --connection-type eice
aws ec2-instance-connect open-tunnel --instance-id i-0dup --local-port 2222
vim notes about open-tunnel.txt
`

func TestParseTunnels(t *testing.T) {
	got := ParseTunnels(psSample)
	want := []LocalTunnel{
		{Port: 2222, RemotePort: 22, InstanceID: "i-0abc"},
		{Port: 2223, RemotePort: 22, InstanceID: "i-0def"},
		{Port: 2224, RemotePort: 22, PrivateIP: "10.0.1.9"},
		{Port: 3306, RemotePort: 3306, InstanceID: "i-0ghi"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tunnel %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTunnelMatchesOnlySSHPortOfTheHost(t *testing.T) {
	h := provider.Host{ID: "i-0abc", PrivateIP: "10.0.1.9"}
	cases := map[string]struct {
		t    LocalTunnel
		want bool
	}{
		"by instance id": {LocalTunnel{Port: 1, RemotePort: 22, InstanceID: "i-0abc"}, true},
		"by private ip":  {LocalTunnel{Port: 1, RemotePort: 22, PrivateIP: "10.0.1.9"}, true},
		"other instance": {LocalTunnel{Port: 1, RemotePort: 22, InstanceID: "i-0zzz"}, false},
		"not ssh port":   {LocalTunnel{Port: 1, RemotePort: 3306, InstanceID: "i-0abc"}, false},
		"no identity":    {LocalTunnel{Port: 1, RemotePort: 22}, false},
	}
	for name, c := range cases {
		if got := c.t.Matches(h); got != c.want {
			t.Errorf("%s: got %v", name, got)
		}
	}
	if got := TunnelsFor(ParseTunnels(psSample), h); len(got) != 2 {
		t.Fatalf("TunnelsFor: %+v", got)
	}
}

func TestDetectTunnelsNeverFails(_ *testing.T) {
	_ = DetectTunnels(context.Background()) // best effort: must not panic or block
}
