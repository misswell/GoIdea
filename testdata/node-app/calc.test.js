// The jest fixture exercises the js test pipeline.

function add(a, b) {
  return a + b;
}

describe('calc', () => {
  test('adds ok', () => {
    expect(add(2, 3)).toBe(5);
  });

  test('adds negatives', () => {
    expect(add(-1, -2)).toBe(-3);
  });

  test('fails on purpose', () => {
    // The rerun-failed regression derives the -t filter from this test's
    // failure and reruns only it.
    expect(add(1, 1)).toBe(3);
  });
});
