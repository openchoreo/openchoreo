import {
  createUnifiedTheme,
} from '@backstage/theme';
import Gilmer from '../assets/fonts/Gilmer/gilmer-regular.woff2';

const GilmerFont = {
  fontFamily: 'Gilmer',
  fontStyle: 'normal',
  src: `
    local('Gilmer'),
    url(${Gilmer}) format('woff2'),
  `,
};
export const openChoreoTheme = createUnifiedTheme({
  fontFamily: 'Gilmer',
  palette: {
    primary: {
      light: '#a6B3ff',
      main: '#5567d5',
      dark: '#4d5ec0',
    },
    secondary: {
      light: '#f7f8fb',
      main: '#8d91a3',
      dark: '#40404b',
    },
    error: {
      light: '#fceded',
      main: '#fe523c',
      dark: '#d64733',
    },
    warning: {
      light: '#fff5eb',
      main: '#ff9d52',
      dark: '#ff9133',
    },
    success: {
      light: '#effdf2',
      main: '#36b475',
      dark: '#05a26b',
    },
    grey: {
      100: '#e6e7ec',
      200: '#cbcedb',
    },
    common: {
      black: '#1d2028',
      white: '#ffffff',
    },
    // Backstage-specific palette additions
    status: {
      ok: '#36b475',
      warning: '#ff9d52',
      error: '#fe523c',
      pending: '#8d91a3',
      running: '#5567d5',
      aborted: '#40404b',
    },
    border: '#e6e7ec',
    textContrast: '#1d2028',
    textVerySubtle: '#8d91a3',
    textSubtle: '#40404b',
    highlight: '#5567d5',
    errorBackground: '#fceded',
    warningBackground: '#fff5eb',
    infoBackground: '#f0f1fb',
    errorText: '#d64733',
    infoText: '#5567d5',
    warningText: '#ff9133',
    linkHover: '#4d5ec0',
    link: '#5567d5',
    gold: '#ff9d52',
    navigation: {
      background: '#ffffff',
      indicator: '#5567d5',
      color: '#1d2028',
      selectedColor: '#5567d5',
      navItem: {
        hoverBackground: '#f7f8fb',
      },
      submenu: {
        background: '#f7f8fb',
      },
    },
    tabbar: {
      indicator: '#5567d5',
    },
    bursts: {
      fontColor: '#1d2028',
      slackChannelText: '#8d91a3',
      backgroundColor: {
        default: '#f7f8fb',
      },
      gradient: {
        linear: 'linear-gradient(135deg, #5567d5 0%, #a6B3ff 100%)',
      },
    },
    pinSidebarButton: {
      icon: '#8d91a3',
      background: '#f7f8fb',
    },
    banner: {
      info: '#5567d5',
      error: '#fe523c',
      text: '#1d2028',
      link: '#5567d5',
      closeButtonColor: '#8d91a3',
      warning: '#ff9d52',
    },
    code: {
      background: '#f7f8fb',
    },
  },
  typography: {
    fontFamily: 'Gilmer',
    htmlFontSize: 16,
    h1: {
      fontSize: 43,
      fontWeight: 700,
      marginBottom: 16,
    },
    h2: {
      fontSize: 29,
      fontWeight: 700,
      marginBottom: 12,
    },
    h3: {
      fontSize: 22,
      fontWeight: 600,
      marginBottom: 8,
    },
    h4: {
      fontSize: 15,
      fontWeight: 600,
      marginBottom: 8,
    },
    h5: {
      fontSize: 13,
      fontWeight: 600,
      marginBottom: 8,
    },
    h6: {
      fontSize: 12,
      fontWeight: 600,
      marginBottom: 8,
    },
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        '@font-face': [GilmerFont],
      },
    },
    MuiButton: {
      styleOverrides: {
        root: {
          textTransform: 'none',
          fontWeight: 400,
        },
      },
    },
  },
});