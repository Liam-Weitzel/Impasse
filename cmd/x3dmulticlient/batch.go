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
		as := batch{convert(x)}
	send:
		for {
			select {
			case x := <-in:
				//log.Println("A: second event")
				as = append(as, convert(x))
			case out <- as:
				//log.Println("A: send success")
				break send
			}
		}
	}
}
