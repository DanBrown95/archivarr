// App breakpoints, in pixels. These mirror naive-ui's own defaults (s: 640,
// m: 1024) so the n-grid responsive spans, our VueUse breakpoint checks, and the
// `max-width` media queries in component styles all line up on the same scale.
// "Mobile" throughout the app means narrower than `s` (i.e. below 640px).
export const breakpoints = {
  s: 640,
  m: 1024,
}
