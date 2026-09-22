/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'ui-sans-serif', 'system-ui', 'Segoe UI', 'Arial', 'sans-serif'],
      },
      colors: {
        ink: '#17202A',
        paper: '#F7F8FA',
        line: '#D7DEE8',
        science: '#1F5E7A',
        graphite: '#3C4654',
        moss: '#43705A',
        signal: '#B56B2A',
      },
      boxShadow: {
        panel: '0 12px 34px rgba(23, 32, 42, 0.08)',
      },
    },
  },
  plugins: [],
};
