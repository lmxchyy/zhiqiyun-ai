/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,jsx}"],
  theme: {
    extend: {
      colors: {
        zhiqi: {
          purple: "#423499",
          "purple-dark": "#30236F",
          "purple-soft": "#F2F0FF",
          orange: "#FF771B",
          ink: "#111827",
          muted: "#667085",
          line: "#E4E7EC",
          canvas: "#F7F8FC",
        },
      },
      fontFamily: {
        display: ['"Plus Jakarta Sans"', '"Noto Sans SC"', "sans-serif"],
        body: ['"Be Vietnam Pro"', '"Noto Sans SC"', "sans-serif"],
      },
      borderRadius: {
        card: "16px",
      },
      boxShadow: {
        soft: "0 16px 40px rgba(27, 24, 56, 0.08)",
      },
    },
  },
  plugins: [],
};
