// +build !windows

package shell

/*
 * We don't allow noPTY mode for non-windows agents.
 * Also, on non-windows agents, we handle job termination
 * by closing the TTY associated with the job.
 * Therefore, all the implementations here are empty.
 */

func (p *Process) setup() {

}

func (p *Process) afterCreation() error {
	return nil
}

func (p *Process) Terminate() error {
	return nil
}
