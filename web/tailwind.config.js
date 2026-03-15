/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./*.{html,js}",
    "./components/**/*.{html,js,jsx,ts,tsx}",
    // local dev: relative path from web/
    "../internal/views/**/*.templ",
    "../internal/handlers/**/*.templ",
    // Docker css stage: templates copied to /src/internal/
    "/src/internal/views/**/*.templ",
    "/src/internal/handlers/**/*.templ",
  ],
  theme: {
    extend: {},
  },
  plugins: [
    require('daisyui'),
  ],
  daisyui: {
    themes: ["dark"],
  },
}
