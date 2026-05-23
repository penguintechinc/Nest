/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        'bg-primary': '#0f172a',
        'bg-secondary': '#1e293b',
        'text-gold': '#fbbf24',
        'text-gold-dim': '#f59e0b',
        'accent-blue': '#0ea5e9',
        'border-slate': '#334155',
      },
    },
  },
  plugins: [],
};
