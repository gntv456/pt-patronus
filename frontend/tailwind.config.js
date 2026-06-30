/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{vue,ts,js}'],
  theme: {
    extend: {
      colors: {
        pt: {
          primary: '#6c5ce7',
          primary2: '#4285f4',
          success: '#2ecc71',
          danger: '#e74c3c',
          warn: '#f39c12',
        },
      },
      borderRadius: { xl: '14px', '2xl': '20px' },
      backdropBlur: { glass: '20px' },
    },
  },
  plugins: [],
}
