import { makeStyles } from '@material-ui/core/styles';

export const useStyles = makeStyles(theme => ({
  pageContent: {
    padding: theme.spacing(3),
    background: 'linear-gradient(135deg, #f8fafc 0%, #e2e8f0 100%)',
    minHeight: '100vh',
  },
  loadingContainer: {
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    alignItems: 'center',
    height: '50vh',
    gap: theme.spacing(2),
  },
  errorContainer: {
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    alignItems: 'center',
    height: '50vh',
    color: theme.palette.error.main,
    gap: theme.spacing(2),
  },
}));
