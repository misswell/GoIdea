"""The pytest fixture exercises the pytest output pipeline."""


def add(a, b):
    return a + b


def test_add_ok():
    assert add(2, 3) == 5


def test_add_negative():
    assert add(-1, -2) == -3


def test_fails_on_purpose():
    # The rerun-failed regression derives the node id filter from this
    # test's failure and reruns only it.
    assert add(1, 1) == 3
