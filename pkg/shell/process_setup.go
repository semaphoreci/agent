// +build !windows

package shell

/*
 * For non-windows agents, we handle job termination
 * by closing the TTY associated with the job.
 * Therefore, no special handling here is necessary.
 */

func (p *Process) setup() {

}

func (p *Process) afterCreation(jobObject uintptr) error {
	return nil
}
