import { createUnifiedTheme, createBaseThemeOptions } from '@backstage/theme';
import Ubuntu from '../assets/fonts/Ubuntu/UbuntuSans-VariableFont_wdth,wght.ttf';
import { alpha } from '@material-ui/core';

// Color constants for reuse
const colors = {
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
  indigo: {
    100: '#f0f1fb',
    200: '#ccd1f2',
  },
  common: {
    black: '#1d2028',
    white: '#ffffff',
  },
};

const UbuntuFont = {
  fontFamily: 'Ubuntu Sans',
  fontStyle: 'normal',
  fontWeight: '100 800',
  fontStretch: '75% 100%',
  fontDisplay: 'swap',
  src: `
    local('Ubuntu Sans'),
    local('UbuntuSans-VariableFont'),
    url(${Ubuntu}) format('truetype')
  `,
};

export const openChoreoTheme = createUnifiedTheme({
  ...createBaseThemeOptions({
    fontFamily: 'Ubuntu Sans',
    palette: {
      ...colors,
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
      fontFamily:
        'Ubuntu Sans, Ubuntu, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
      htmlFontSize: 15,
      h1: {
        fontSize: 43,
        fontWeight: 700,
        marginBottom: 0,
      },
      h2: {
        fontSize: 29,
        fontWeight: 700,
        marginBottom: 0,
      },
      h3: {
        fontSize: 22,
        fontWeight: 600,
        marginBottom: 0,
      },
      h4: {
        fontSize: 16,
        fontWeight: 600,
        marginBottom: 0,
      },
      h5: {
        fontSize: 14,
        fontWeight: 600,
        marginBottom: 0,
      },
      h6: {
        fontSize: 13,
        fontWeight: 600,
        marginBottom: 0,
      },
    },
  }),
  components: {
    BackstageHeader: {
      styleOverrides: {
        header: {
          backgroundColor: colors.primary.main,
          backgroundImage: 'none',
          height: 98,
        },
        title: {
          fontSize: 25,
        },
      },
    },
    BackstageItemCardHeader: {
      styleOverrides: {
        root: {
          backgroundColor: '#5567d5',
          backgroundImage: 'none!important',
        },
      },
    },
    BackstageSidebarItem: {
      styleOverrides: {
        label: {
          fontWeight: 500,
        },
      },
    },
    CatalogReactEntityDisplayName: {
      styleOverrides: {
        root: {
          paddingBottom: 8,
          paddingTop: 8,
        },
      },
    },
    MuiCssBaseline: {
      styleOverrides: {
        '@font-face': [UbuntuFont],
        'body, html': {
          fontFamily:
            'Ubuntu Sans, Ubuntu, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif !important',
        },
        '[class*="BackstageSidebarDivider-root"]': {
          opacity: 0.2,
        },
        // SVG elements in the entity relations diagram
        'g[data-testid="node"] rect': {
          '&.primary': {
            fill: `${colors.primary.dark} !important`,
            stroke: `${colors.primary.dark} !important`,
            strokeWidth: '2px !important',
          },
          '&.secondary': {
            fill: `${colors.secondary.dark} !important`,
            stroke: `${colors.secondary.dark} !important`,
            strokeWidth: '2px !important',
          },
        },
        'g[data-testid="node"] text': {
          fill: `${colors.common.white} !important`,
        },
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
    MuiTypography: {
      styleOverrides: {
        body1: {
          fontSize: 14,
          fontWeight: 'normal',
          fontStretch: 'normal',
          fontStyle: 'normal',
          lineHeight: 1.53846154,
          letterSpacing: 'normal',
        },
        body2: {
          fontSize: 13,
          fontWeight: 'normal',
          fontStretch: 'normal',
          fontStyle: 'normal',
          lineHeight: 1.33333,
          letterSpacing: 'normal',
        },
      },
    },
    BackstageTableHeader: {
      styleOverrides: {
        header: {
          textTransform: 'none',
          color: '#8d91a3!important',
          fontWeight: 500,
          fontSize: 14,
          borderTop: 'none',
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          padding: '8px 16px 8px 20px !important',
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: 8,
        },
      },
    },
    MuiTextField: {
      defaultProps: {
        variant: 'outlined',
      },
      styleOverrides: {
        root: {
          '&:hover .MuiOutlinedInput-notchedOutline': {
            borderColor: `${colors.primary.light} !important`,
          },
        },
      },
    },
    MuiFormControl: {
      defaultProps: {
        variant: 'outlined',
      },
    },
    MuiSelect: {
      defaultProps: {
        variant: 'outlined',
      },
    },
    MuiInput: {
      styleOverrides: {
        root: {
          backgroundColor: colors.secondary.light,
          border: `1px solid transparent`,
          transition: 'all 0.3s',
          borderRadius: 4,
          padding: '2px 4px',
          color: 'inherit',
          fontSize: 13,
          '&:before': {
            display: 'none',
          },
          '&:after': {
            display: 'none',
          },
          '&:hover:not(.Mui-disabled):before': {
            display: 'none',
          },
          '&:hover:not(.Mui-focused)': {
            borderColor: colors.indigo[200],
          },
          '&.Mui-focused': {
            borderColor: colors.primary.light,
          },
        },
      },
    },
    MuiOutlinedInput: {
      styleOverrides: {
        notchedOutline: {
          borderColor: colors.grey[100],
          boxShadow: `0 1px 2px -1px ${alpha(
            colors.common.black,
            0.08,
          )}, 0 -3px 9px 0 ${alpha(colors.common.black, 0.04)} inset`,
          borderRadius: 5,
          '$root:hover &': {
            borderColor: `${colors.indigo[200]} `,
          },
          '$root.Mui-focused &': {
            borderColor: `${colors.primary.light}`,
            borderWidth: '1px',
          },
          '$root.Mui-error &': {
            borderColor: `${colors.error.main}`,
          },
        },
      },
    },
    MuiIconButton: {
      styleOverrides: {
        root: {
          padding: 4,
          fontSize: 'inherit',
        },
      },
    },
  },
});
