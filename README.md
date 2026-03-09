# sparkwalk

Fast parallel, Linux/amd64-only alternative to `filepath.Walk`.

import `"github.com/sparkedhost/sparkwalk"`

Performs traversals in parallel using 16 worker goroutines. The result is roughly 4–6× the traversal rate of the standard `filepath.Walk`. The two are not identical since `walkFn` is called concurrently from multiple goroutines. Take note of the following:

1. This walk honors all of the `WalkFunc` error semantics. When multiple goroutines simultaneously decide to stop traversal, only the **first** error is returned from `Walk`.

2. A few additional `walkFn` calls may occur after the first error-generating call, because other goroutines may already have files in flight. Design your `walkFn` accordingly (e.g. stop accumulating results once an error is returned).

3. `walkFn` is called concurrently from multiple goroutines and must be safe for concurrent use. Use a channel or a mutex-protected variable to accumulate results.

4. The sentinel value for skipping a directory is `walk.ErrSkipDir` (note: `ErrSkipDir`, not `SkipDir`).

5. Partial directory reads (e.g. from overlay2 whiteout entries returning `EBADMSG`) are handled gracefully: if `walkFn` returns `nil` for the error, traversal continues over any entries that were read before the failure.

There is a test file covering the same cases as `path/filepath` in the Go standard library.

Copyright (c) 2016 Michael T Jones

Copyright (c) 2026 SparkedHostLLC

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
