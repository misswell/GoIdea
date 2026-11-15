//! The cargo test fixture exercises the cargo test pipeline.

/// Adds two numbers.
pub fn add(a: i32, b: i32) -> i32 {
    a + b
}

#[cfg(test)]
mod tests {
    use super::add;

    #[test]
    fn it_adds_ok() {
        assert_eq!(add(2, 3), 5);
    }

    #[test]
    fn it_adds_negatives() {
        assert_eq!(add(-1, -2), -3);
    }

    #[test]
    fn it_fails_on_purpose() {
        // The rerun-failed regression derives the `--exact` filter from this
        // test's failure and reruns only it.
        assert_eq!(add(1, 1), 3, "expected the fixture failure");
    }
}
