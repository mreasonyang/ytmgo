//! WebView2 still needs a writable UDF for an InPrivate controller. Its executable
//! can be in Program Files, so never let WebView2 choose its executable-adjacent UDF.
use std::path::PathBuf;
use std::time::{Duration, Instant};

pub struct Profile {
    root: Option<tempfile::TempDir>,
}

impl Profile {
    pub fn new() -> std::io::Result<Self> {
        Ok(Self {
            root: Some(
                tempfile::Builder::new()
                    .prefix("ytmgo-playback-host-")
                    .tempdir()?,
            ),
        })
    }

    pub fn path(&self) -> PathBuf {
        self.root
            .as_ref()
            .expect("profile is live")
            .path()
            .to_path_buf()
    }

    // Called after both WebView and WebContext drop. WebView2 may briefly retain a
    // filesystem handle while its browser process exits. Never wait indefinitely.
    pub fn cleanup(&mut self) -> bool {
        let Some(root) = self.root.take() else {
            return true;
        };
        let deadline = Instant::now() + Duration::from_secs(3);
        let removed = loop {
            match std::fs::remove_dir_all(root.path()) {
                Ok(()) => break true,
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => break true,
                Err(_) if Instant::now() < deadline => {
                    std::thread::sleep(Duration::from_millis(100))
                }
                Err(_) => break false,
            }
        };
        drop(root);
        removed
    }
}

impl Drop for Profile {
    fn drop(&mut self) {
        let _ = self.cleanup();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn separate_helpers_get_distinct_writable_temporary_profiles_and_remove_them() {
        let mut first = Profile::new().unwrap();
        let second = Profile::new().unwrap();
        let first_path = first.path();
        let second_path = second.path();
        assert_ne!(first_path, second_path);
        std::fs::write(first_path.join("synthetic"), b"fixture").unwrap();
        assert!(first.cleanup());
        assert!(!first_path.exists());
        drop(second);
        assert!(!second_path.exists());
    }
}
