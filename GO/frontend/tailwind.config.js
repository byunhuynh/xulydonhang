/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: '#0f0e1f',
        panel: '#1e1c3b',
        border: '#2c2853',
        ink: '#F1F0FB',
        muted: '#9A95C4',
        accent: '#28C5F2',
        brandPurple: '#8B89D6',
        success: '#35D68A',
        warning: '#FFCF5C',
        danger: '#FF6B81',
      },
      fontFamily: {
        sans: ['"Be Vietnam Pro"', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'monospace'],
      },
      keyframes: {
        'pulse-glow': {
          '0%, 100%': { boxShadow: '0 0 0 0 rgba(40,197,242,0.35), 0 4px 18px -4px rgba(40,197,242,0.5)' },
          '50%': { boxShadow: '0 0 0 8px rgba(40,197,242,0), 0 4px 18px -4px rgba(40,197,242,0.5)' },
        },
        rise: {
          from: { opacity: '0', transform: 'translateY(8px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        'flash-cell': {
          '0%': { backgroundColor: 'rgba(40,197,242,0.35)' },
          '100%': { backgroundColor: 'transparent' },
        },
      },
      animation: {
        'pulse-glow': 'pulse-glow 2.6s ease-in-out infinite',
        rise: 'rise 0.4s cubic-bezier(0.16,1,0.3,1) both',
        'flash-cell': 'flash-cell 0.9s ease',
      },
    },
  },
  plugins: [],
}
