package seccomp

import (
	"runtime"

	"github.com/pkg/errors"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/go-seccomp-bpf"
)

var (
	defaultPolicy    *seccomp.Policy
	registeredPolicy *seccomp.Policy
)

// MustRegisterPolicy registers a seccomp policy to use instead of the default
// policy. This can be used to register an application specific seccomp policy
// that is tailored to the specific system calls that the application requires.
// It panics if a policy has already been registered or if the given policy
// is invalid.
func MustRegisterPolicy(p *seccomp.Policy) {
	if p == nil {
		panic(errors.New("seccomp policy cannot be nil"))
	}

	if registeredPolicy != nil {
		panic(errors.New("a seccomp policy is already registered"))
	}

	// Ensure that the policy is valid and usable.
	if _, err := p.Assemble(); err != nil {
		panic(errors.Wrap(err, "failed to register seccomp policy"))
	}
	registeredPolicy = p
}

// LoadFilter loads a seccomp system call filter into the kernel for this
// process. This feature is only available on Linux 3.17+. If c is nil or does
// not contain a seccomp policy then a default policy will be used.
//
// An error is returned if there is a config validation problem. Otherwise any
// errors interfacing with the kernel are logged (i.e. it is non-fatal if
// seccomp cannot be setup).
//
// Policy precedence order (highest to lowest):
// - Policy values from config
// - Application registered policy
// - Default policy (a simple blacklist)
func LoadFilter(c *common.Config) error {
	// Bail out if seccomp.enabled=false.
	if c != nil && !c.Enabled() {
		return nil
	}

	p, err := getPolicy(c)
	if err != nil {
		return err
	}

	loadFilter(p)
	return nil
}

// loadFilter loads a system call filter.
func loadFilter(p *seccomp.Policy) {
	log := logp.NewLogger("seccomp")

	if runtime.GOOS != "linux" {
		log.Debug("Syscall filtering is only supported on Linux")
		return
	}

	if !seccomp.Supported() {
		log.Info("Syscall filter could not be installed because the kernel " +
			"does not support seccomp")
		return
	}

	if p == nil {
		log.Debug("No seccomp policy is defined")
		return
	}

	filter := seccomp.Filter{
		NoNewPrivs: true,
		Flag:       seccomp.FilterFlagTSync,
		Policy:     *p,
	}

	log.Debugw("Loading syscall filter", "seccomp_filter", filter)
	if err := seccomp.LoadFilter(filter); err != nil {
		log.Warn("Syscall filter could not be installed", "error", err,
			"seccomp_filter", filter)
		return
	}

	log.Infow("Syscall filter successfully installed")
}

func getPolicy(c *common.Config) (*seccomp.Policy, error) {
	policy := defaultPolicy
	if registeredPolicy != nil {
		policy = registeredPolicy
	}

	if c != nil && (c.HasField("default_action") || c.HasField("syscalls")) {
		if policy == nil {
			policy = &seccomp.Policy{}
		}

		if err := c.Unpack(policy); err != nil {
			return nil, err
		}
	}

	return policy, nil
}