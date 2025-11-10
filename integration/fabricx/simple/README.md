# the simple test

`simple` is yet another test to verify the fabricx support.
It provides a traditional integration (`simple_test.go`) test and a network app (`app/main.go`), which can be used to spawn a network and do some manual interactive testing.

Usage:

Start a local `simple` network instance. You can find the network topology in `topo.go`.

```bash
export FAB_BINS=/the/path/to/your/fabins
cd integration/fabricx/simple/app;
go run . network start
```

Once the network is up and running you can interact with it.
