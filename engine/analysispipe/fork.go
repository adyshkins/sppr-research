package analysispipe

// Fork enables an atomic extension by downstream forecast stages without
// modifying the v0.3 processing semantics or recorded replay results.
func (p *Pipeline) Fork() *Pipeline {
	return &Pipeline{config: CloneConfig(p.config), monitor: p.monitor.Fork()}
}
