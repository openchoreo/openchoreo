import { makeStyles } from '@material-ui/core/styles';

export const useStyles = makeStyles(theme => ({
  dataplaneDetailsSection: {
    marginTop: theme.spacing(4),
  },
  dataplaneDetailTitle: {
    fontSize: '1.25rem',
    fontWeight: 600,
    color: '#374151',
    marginBottom: theme.spacing(3),
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  dataplaneCard: {
    borderRadius: 12,
    border: '1px solid #e2e8f0',
    boxShadow:
      '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
    background: 'white',
    marginBottom: theme.spacing(3),
    overflow: 'hidden',
  },
  dataplaneHeader: {
    padding: theme.spacing(2, 3),
    borderBottom: '1px solid #f3f4f6',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    background: '#f9fafb',
    cursor: 'pointer',
  },
  dataplaneTitle: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    fontSize: '1.125rem',
    fontWeight: 600,
    color: '#374151',
  },
  dataplaneIcon: {
    fontSize: '1.25rem',
    color: '#6b7280',
  },
  statusChip: {
    backgroundColor: '#f3f4f6',
    color: '#6b7280',
    fontWeight: 500,
    fontSize: '0.75rem',
  },
  environmentsSection: {
    padding: theme.spacing(3),
  },
  sectionTitle: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    marginBottom: theme.spacing(2),
    fontSize: '1rem',
    fontWeight: 500,
    color: '#6b7280',
  },
  environmentGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
    gap: theme.spacing(2),
    marginTop: theme.spacing(2),
  },
  environmentCard: {
    borderRadius: 8,
    border: '1px solid #e5e7eb',
    background: 'white',
    transition: 'all 0.2s ease',
    height: '100%',
    '&:hover': {
      borderColor: '#d1d5db',
      boxShadow: '0 1px 3px rgba(0, 0, 0, 0.1)',
    },
  },
  environmentCardProduction: {
    borderColor: theme.palette.success.main,
  },
  environmentCardNonProduction: {
    borderColor: theme.palette.info.main,
  },
  environmentHeader: {
    padding: theme.spacing(2),
    borderBottom: '1px solid #f3f4f6',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'flex-start',
  },
  environmentName: {
    fontSize: '1rem',
    fontWeight: 600,
    color: '#111827',
    marginBottom: theme.spacing(0.5),
  },
  environmentChip: {
    fontSize: '0.75rem',
    height: 24,
    fontWeight: 500,
  },
  productionChip: {
    backgroundColor: theme.palette.success.light,
    color: theme.palette.success.dark,
    border: `1px solid ${theme.palette.success.main}`,
  },
  nonProductionChip: {
    backgroundColor: theme.palette.secondary.light,
    color: theme.palette.primary.dark,
    border: `1px solid ${theme.palette.primary.main}`,
  },
  environmentContent: {
    padding: 12,
  },
  environmentDetail: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 8,
    '&:last-child': {
      marginBottom: 0,
    },
  },
  environmentLabel: {
    fontSize: '0.875rem',
    color: '#6b7280',
    fontWeight: 500,
  },
  environmentValue: {
    fontSize: '0.875rem',
    color: '#374151',
    fontWeight: 600,
  },
  componentCount: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    backgroundColor: '#eff6ff',
    color: '#1e40af',
    padding: '2px 8px',
    borderRadius: 8,
    fontSize: '0.75rem',
    fontWeight: 600,
  },
  emptyState: {
    textAlign: 'center',
    padding: theme.spacing(4),
    color: '#6b7280',
    fontStyle: 'italic',
  },
}));
