import { makeStyles } from '@material-ui/core/styles';

export const useStyles = makeStyles(theme => ({
  searchSection: {
    display: 'flex',
    marginBottom: theme.spacing(4),
    justifyContent: 'center',
    width: '100%',
  },
  searchPaper: {
    padding: theme.spacing(1, 2),
    display: 'flex',
    alignItems: 'center',
    width: theme.spacing(100),
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.spacing(20),
    cursor: 'pointer',
    transition: 'all 0.2s ease-in-out',
    '&:hover': {
      borderColor: theme.palette.primary.main,
      boxShadow: theme.shadows[2],
    },
    '&:focus-within': {
      borderColor: theme.palette.primary.main,
      boxShadow: `0 0 0 2px ${theme.palette.primary.main}25`,
    },
  },
  searchPlaceholder: {
    flex: 1,
    fontSize: '1rem',
    color: theme.palette.text.secondary,
    marginLeft: theme.spacing(1),
    cursor: 'pointer',
  },
  searchModal: {
    '& .MuiDialog-paper': {
      borderRadius: theme.shape.borderRadius * 2,
      maxHeight: '80vh',
    },
  },
  searchModalContent: {
    padding: theme.spacing(2),
    paddingBottom: theme.spacing(1),
  },
  searchModalHeader: {
    display: 'flex',
    alignItems: 'center',
    width: '100%',
    gap: theme.spacing(2),
    marginBottom: theme.spacing(2),
    '& .MuiInputBase-root': {
      flex: 1,
      backgroundColor: theme.palette.background.default,
      borderRadius: theme.shape.borderRadius,
      padding: theme.spacing(1, 2),
    },
  },
  searchModalClose: {
    cursor: 'pointer',
    color: theme.palette.text.secondary,
    '&:hover': {
      color: theme.palette.text.primary,
    },
  },
  searchModalResults: {
    maxHeight: '60vh',
    overflow: 'auto',
    '& .MuiList-root': {
      padding: 0,
    },
    '& .MuiListItem-root': {
      borderRadius: theme.shape.borderRadius,
      marginBottom: theme.spacing(0.5),
      '&:hover': {
        backgroundColor: theme.palette.action.hover,
      },
    },
  },
  overviewSection: {
    marginBottom: theme.spacing(4),
  },
  widgetContainer: {
    display: 'flex',
    gap: theme.spacing(3),
    marginTop: theme.spacing(3),
    flexWrap: 'wrap',
    '& > *': {
      flex: '1 1 300px',
      minWidth: '300px',
    },
    [theme.breakpoints.down('sm')]: {
      flexDirection: 'column',
      '& > *': {
        flex: '1 1 auto',
        minWidth: 'auto',
      },
    },
  },
  platformDetailsSection: {
    marginBottom: theme.spacing(4),
  },
  sidebarSection: {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(3),
    marginTop: theme.spacing(3),
  },
  sidebarTitle: {
    marginBottom: theme.spacing(1),
  },
  sidebarWidget: {
    width: '100%',
    display: 'flex',
    justifyContent: 'flex-end',
    '& > *': {
      height: '100%',
      width: '100%',
    },
  },
  quickActionsContainer: {
    marginTop: theme.spacing(3),
  },
  quickActionCard: {
    height: '100%',
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.spacing(1),
    transition: 'all 0.2s ease-in-out',
    '&:hover': {
      borderColor: theme.palette.primary.main,
      boxShadow: theme.shadows[4],
      transform: 'translateY(-2px)',
    },
  },
  quickActionCardAction: {
    height: '100%',
    display: 'flex',
    alignItems: 'flex-start',
    textDecoration: 'none',
    '&:hover': {
      textDecoration: 'none',
    },
  },
  quickActionCardContent: {
    width: '100%',
    padding: theme.spacing(3),
  },
  quickActionHeader: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: theme.spacing(1),
  },
  quickActionTitle: {
    fontWeight: 600,
  },
  quickActionIcon: {
    color: theme.palette.text.secondary,
    opacity: 0.6,
  },
  searchBarInput: {
    maxWidth: '60vw',
    margin: 'auto',
    backgroundColor: theme.palette.background.paper,
    borderRadius: '50px',
    boxShadow: theme.shadows[1],
  },
  searchBarOutline: {
    borderStyle: 'none',
  },
  welcomeCard: {
    minHeight: '200px',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
  },
  groupBadge: {
    display: 'inline-block',
    padding: '4px 12px',
    margin: '4px',
    borderRadius: '12px',
    backgroundColor: theme.palette.primary.main,
    color: theme.palette.primary.contrastText,
    fontSize: '0.875rem',
  },
}));
