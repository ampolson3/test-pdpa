// A stand-in for a real analytics tag (e.g. Google Analytics) — sets a cookie the instant it runs, so the
// E2E test can tell whether it ran at all just by reading document.cookie.
document.cookie = '_fake_ga=1; path=/';
