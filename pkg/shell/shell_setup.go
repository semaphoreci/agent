// +build !windows

package shell

/*
 * For non-windows agents, we handle job termination
 * by closing the TTY associated with the job.
 * Therefore, no special handling here is necessary.
 */

func (s *Shell) Setup() {

}

func (s *Shell) Terminate() error {
	return nil
}
