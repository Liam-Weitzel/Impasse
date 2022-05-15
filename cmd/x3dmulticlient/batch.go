package main

type batch []func(*client)

func (b batch) run(c *client) {
	for _, fn := range b {
		fn(c)
	}
}

func batching[T any](
	in <-chan T,
	out chan batch,
	convert func(T) func(*client),
) {
	defer close(out)

	for {
		x, ok := <-in
		if !ok {
			break
		}
		fn := convert(x)
		if fn == nil {
			continue
		}
		as := batch{fn}
	send:
		for {
			select {
			case x := <-in:
				if fn := convert(x); fn != nil {
					as = append(as, fn)
				}
			case out <- as:
				break send
			}
		}
	}
}
