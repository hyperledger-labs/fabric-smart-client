# FSC Benchmarks

This package contains useful micro benchmarks for the FSC runtime.

## Benchmarking

### Background material

There are many useful articles about go benchmarks available online. Here just a few good starting points: 
- https://gobyexample.com/testing-and-benchmarking
- https://blog.cloudflare.com/go-dont-collect-my-garbage/
- https://mcclain.sh/posts/go-benchmarking/

### Run them all

We can run all benchmarks in this package as follows:

```bash
go test -bench=. -benchmem -count=10 -timeout=20m -cpu=1,2,4,8,16 -run=^$ ./... > plots/benchmark_gc_100.txt
```

#### Garbage Collection

Some code, in particular, allocation-intensive operations may benefit from tweaking the garbage collector settings.
There is a highly recommended read about go's GC https://go.dev/doc/gc-guide.

```bash
# Default
GOGC=100

# No garbage collection, use this setting only for testing! :)
GOGC=off
```

If we want to study the impact of different GC settings we can run the following, for example:

```bash
GOGC=100 go test -bench=. -benchmem -count=10 -timeout=20m -cpu=1,2,4,6,8,10,12,14,16,18,20,22,24,26,28,30,32,34,36,38,40,42,48,64 -run=^$ ./... > plots/benchmark_gc_100.txt
GOGC=off go test -bench=. -benchmem -count=10 -timeout=20m -cpu=1,2,4,6,8,10,12,14,16,18,20,22,24,26,28,30,32,34,36,38,40,42,48,64 -run=^$ ./... > plots/benchmark_gc_100.txt
GOGC=8000 go test -bench=. -benchmem -count=10 -timeout=20m -cpu=1,2,4,6,8,10,12,14,16,18,20,22,24,26,28,30,32,34,36,38,40,42,48,64 -run=^$ ./... > plots/benchmark_gc_8000.txt
```


## Plotting

The `plot/` directory contains a python script to visualize the benchmark results.

### Install

```bash
python3 -m venv env
source env/bin/activate
pip3 install -r requirements.txt
```

### Plot

Run the python script and provide the input file and the output file as arguments.

```bash
python3 plot.py benchmark_gc_off.txt benchmark_gc_off.pdf
```

This will generate the graph as pdf (`result_<timestamp>.pdf`).

### Example

Let's run the `ECDSASignView` benchmark as an example.
We turn garbage collection off using with `GOGC=off`, set the number of benchmark iteration with `-count=10`, and set the number of workers with `-cpu=1,2,4,8,16`.
We save the results in `benchmark_gc_off.txt`, which we use later to plot our graphs.

Once the benchmark is finished, we use `plot/plot.py` to create the result graphs as `pdf`.

```bash
GOGC=off go test -bench='ECDSASignView' -benchmem -count=10 -cpu=1,2,4,8,16 -run=^$ ./... > plots/benchmark_gc_off.txt
cd plots; python3 plot.py benchmark_gc_off.txt benchmark_gc_off.pdf
```

Happy benchmarking!
