package servicedrop

import (
	"testing"
	"time"
	// "code.google.com/p/go.crypto/ssh"
)

func TestRSALink(t *testing.T) {
	rs := RSASSHProtocolLink("io", "192.30.252.0", 22, "influx6", "/home/flux/Lab/ssh/github")
	t.Log("rs:", rs)

	go rs.Dial()

	<-time.After(time.Duration(10) * time.Millisecond)

	t.Log("killing connection for:", rs.Descriptor().Host())
	rs.Drop()
}

func TestPasswordLink(t *testing.T) {

	rs := PasswordSSHProtocolLink("io", "192.30.252.0", 22, "influx6", "Loc*20FOrm3")

	t.Log("rs:", rs)

	go rs.Dial()

	<-time.After(time.Duration(10) * time.Millisecond)

	t.Log("killing connection for:", rs.Descriptor().Host())
	rs.Drop()
}
