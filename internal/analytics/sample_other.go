//go:build !windows

package analytics

func (c *Collector) sampleWindowsNet() error {
	return nil
}
