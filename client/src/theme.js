import { createTheme } from '@mui/material/styles';

// Stat card colors tuned per mode so cards stay readable.
const statsByMode = {
  light: { blue: '#1976d2', green: '#2e7d32', orange: '#ed6c02', purple: '#9c27b0' },
  dark: { blue: '#90caf9', green: '#66bb6a', orange: '#ffa726', purple: '#ce93d8' },
};

export function buildTheme(mode = 'light') {
  return createTheme({
    palette: {
      mode,
      primary: {
        main: '#1976d2',
      },
      secondary: {
        main: '#dc004e',
      },
      stats: statsByMode[mode] || statsByMode.light,
      loginGradient: { start: '#667eea', end: '#764ba2' },
    },
    components: {
      MuiButton: {
        styleOverrides: {
          root: {
            textTransform: 'none',
          },
        },
      },
    },
  });
}

export default buildTheme;
