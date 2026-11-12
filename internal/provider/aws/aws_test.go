package aws

import "testing"

func TestUserForImage(t *testing.T) {
	for desc, want := range map[string]string{
		"Canonical, Ubuntu, 22.04 LTS": "ubuntu",
		"Debian 12 (HVM)":              "admin",
		"Amazon Linux 2023 AMI":        "ec2-user",
		"Rocky Linux 9":                "rocky",
		"Red Hat Enterprise Linux 9":   "ec2-user",
		"SUSE Linux Enterprise 15":     "ec2-user",
		"CentOS Stream 9":              "ec2-user",
		"something unknown":            "ec2-user",
	} {
		if got := UserForImage(desc); got != want {
			t.Errorf("%q: got %s want %s", desc, got, want)
		}
	}
}
