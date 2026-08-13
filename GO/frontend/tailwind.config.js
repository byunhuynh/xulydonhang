/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: '#0B0F14',
        panel: '#131A22',
        border: '#232E3A',
        ink: '#E8EDF2',
        muted: '#8A97A6',
        accent: '#3DD9FF',
        success: '#3ED598',
        warning: '#FFC24B',
        danger: '#FF5D5D',
      },
      fontFamily: {
        sans: ['"Be Vietnam Pro"', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'monospace'],
      },
    },
  },
  plugins: [],
}
