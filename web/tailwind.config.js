/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./*.{html,js}",
    "./components/**/*.{html,js,jsx,ts,tsx}",
    "../internal/views/**/*.templ",
    "../internal/handlers/**/*.templ",
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
