import { FC, ChangeEvent } from 'react';
import {
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormGroup,
  FormControlLabel,
  Checkbox,
  Typography,
  Paper,
  Grid,
} from '@material-ui/core';
import { Skeleton } from '@material-ui/lab';
import { makeStyles } from '@material-ui/core/styles';
import {
  RuntimeLogsFilters,
  Environment,
  LOG_LEVELS,
  TIME_RANGE_OPTIONS,
} from './types';

const useStyles = makeStyles(theme => ({
  filterContainer: {
    padding: theme.spacing(2),
    marginBottom: theme.spacing(2),
  },
  filterSection: {
    marginBottom: theme.spacing(2),
  },
  filterTitle: {
    marginBottom: theme.spacing(1),
    fontWeight: 'bold',
  },
  logLevelCheckbox: {
    padding: theme.spacing(0.5),
  },
  errorLevel: {
    color: theme.palette.error.main,
    fontWeight: 500,
  },
  warnLevel: {
    color: theme.palette.warning.main,
    fontWeight: 500,
  },
  infoLevel: {
    color: theme.palette.info.main,
    fontWeight: 500,
  },
  debugLevel: {
    color: theme.palette.text.secondary,
    fontWeight: 500,
  },
  undefinedLevel: {
    color: theme.palette.text.disabled,
    fontWeight: 500,
  },
}));

interface LogsFilterProps {
  filters: RuntimeLogsFilters;
  onFiltersChange: (filters: Partial<RuntimeLogsFilters>) => void;
  environments: Environment[];
  environmentsLoading: boolean;
  disabled?: boolean;
}

export const LogsFilter: FC<LogsFilterProps> = ({
  filters,
  onFiltersChange,
  environments,
  environmentsLoading,
  disabled = false,
}) => {
  const classes = useStyles();

  const handleLogLevelChange = (level: string) => {
    const newLogLevels = filters.logLevel.includes(level)
      ? filters.logLevel.filter(l => l !== level)
      : [...filters.logLevel, level];

    onFiltersChange({ logLevel: newLogLevels });
  };

  const handleEnvironmentChange = (event: ChangeEvent<{ value: unknown }>) => {
    onFiltersChange({ environmentId: event.target.value as string });
  };

  const handleTimeRangeChange = (event: ChangeEvent<{ value: unknown }>) => {
    onFiltersChange({ timeRange: event.target.value as string });
  };

  const getLogLevelClassName = (level: string) => {
    switch (level) {
      case 'ERROR':
        return classes.errorLevel;
      case 'WARN':
        return classes.warnLevel;
      case 'INFO':
        return classes.infoLevel;
      case 'DEBUG':
        return classes.debugLevel;
      case 'UNDEFINED':
        return classes.undefinedLevel;
      default:
        return '';
    }
  };

  return (
    <Paper className={classes.filterContainer}>
      <Grid container spacing={3}>
        <Grid item xs={12} md={4}>
          <div className={classes.filterSection}>
            <Typography variant="subtitle2" className={classes.filterTitle}>
              Log Levels
            </Typography>
            <FormGroup>
              {LOG_LEVELS.map(level => (
                <FormControlLabel
                  key={level}
                  control={
                    <Checkbox
                      checked={filters.logLevel.includes(level)}
                      onChange={() => handleLogLevelChange(level)}
                      disabled={disabled}
                      className={classes.logLevelCheckbox}
                    />
                  }
                  label={
                    <span className={getLogLevelClassName(level)}>{level}</span>
                  }
                />
              ))}
            </FormGroup>
          </div>
        </Grid>

        <Grid item xs={12} md={4}>
          <div className={classes.filterSection}>
            <FormControl fullWidth disabled={disabled || environmentsLoading}>
              <InputLabel>Environment</InputLabel>
              {environmentsLoading ? (
                <Skeleton variant="rect" height={56} />
              ) : (
                <Select
                  value={filters.environmentId}
                  onChange={handleEnvironmentChange}
                >
                  {environments.map(env => (
                    <MenuItem key={env.id} value={env.id}>
                      {env.name}
                    </MenuItem>
                  ))}
                </Select>
              )}
            </FormControl>
          </div>
        </Grid>

        <Grid item xs={12} md={4}>
          <div className={classes.filterSection}>
            <FormControl fullWidth disabled={disabled}>
              <InputLabel>Time Range</InputLabel>
              <Select
                value={filters.timeRange}
                onChange={handleTimeRangeChange}
              >
                {TIME_RANGE_OPTIONS.map(option => (
                  <MenuItem key={option.value} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </div>
        </Grid>
      </Grid>
    </Paper>
  );
};
