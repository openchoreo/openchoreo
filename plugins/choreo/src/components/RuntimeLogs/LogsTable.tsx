import React from 'react';
import {
  Table,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
  Paper,
  Box,
  Typography,
  CircularProgress,
} from '@material-ui/core';
import { Skeleton } from '@material-ui/lab';
import { makeStyles } from '@material-ui/core/styles';
import { LogEntry as LogEntryType } from './types';
import { LogEntry } from './LogEntry';

const useStyles = makeStyles(theme => ({
  tableContainer: {
    maxHeight: '70vh',
    overflow: 'auto',
  },
  table: {
    minWidth: 650,
  },
  headerCell: {
    fontWeight: 'bold',
    backgroundColor: theme.palette.grey[100],
    position: 'sticky',
    top: 0,
    zIndex: 1,
  },
  emptyState: {
    textAlign: 'center',
    padding: theme.spacing(4),
    color: theme.palette.text.secondary,
  },
  loadingContainer: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    padding: theme.spacing(2),
  },
  loadingRow: {
    padding: theme.spacing(2),
  },
  skeletonRow: {
    height: 60,
  },
}));

interface LogsTableProps {
  logs: LogEntryType[];
  loading: boolean;
  hasMore: boolean;
  totalCount: number;
  loadingRef: React.RefObject<HTMLDivElement>;
  onRetry?: () => void;
}

export const LogsTable: React.FC<LogsTableProps> = ({
  logs,
  loading,
  hasMore,
  totalCount,
  loadingRef,
  onRetry,
}) => {
  const classes = useStyles();

  const renderLoadingSkeletons = () => {
    return Array.from({ length: 5 }).map((_, index) => (
      <TableRow key={`skeleton-${index}`}>
        <TableCell>
          <Skeleton variant="text" width="100%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="rect" width={60} height={24} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="100%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={80} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={100} />
        </TableCell>
        <TableCell>
          <Skeleton variant="rect" width={24} height={24} />
        </TableCell>
      </TableRow>
    ));
  };

  const renderEmptyState = () => {
    if (loading) {
      return null;
    }

    return (
      <TableRow>
        <TableCell colSpan={6}>
          <Box className={classes.emptyState}>
            <Typography variant="h6" gutterBottom>
              No logs found
            </Typography>
            <Typography variant="body2">
              Try adjusting your filters or time range to see more logs.
            </Typography>
          </Box>
        </TableCell>
      </TableRow>
    );
  };

  return (
    <Paper>
      <Box className={classes.tableContainer}>
        <Table className={classes.table} stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell className={classes.headerCell}>Timestamp</TableCell>
              <TableCell className={classes.headerCell}>Level</TableCell>
              <TableCell className={classes.headerCell}>Message</TableCell>
              <TableCell className={classes.headerCell}>Container</TableCell>
              <TableCell className={classes.headerCell}>Pod</TableCell>
              <TableCell className={classes.headerCell} width={100}>
                Details
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {logs.length === 0 && !loading && renderEmptyState()}

            {logs.length === 0 && loading && renderLoadingSkeletons()}

            {logs.map((log, index) => (
              <LogEntry key={`${log.timestamp}-${index}`} log={log} />
            ))}

            {hasMore && (
              <TableRow>
                <TableCell colSpan={6}>
                  <Box className={classes.loadingContainer} ref={loadingRef}>
                    {loading ? (
                      <Box display="flex" alignItems="center">
                        <CircularProgress size={20} />
                        <Typography variant="body2" style={{ marginLeft: 8 }}>
                          Loading more logs...
                        </Typography>
                      </Box>
                    ) : (
                      <Typography variant="body2" color="textSecondary">
                        Scroll to load more logs
                      </Typography>
                    )}
                  </Box>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Box>

      {logs.length > 0 && (
        <Box
          p={2}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="body2" color="textSecondary">
            Showing {logs.length} of {totalCount} logs
          </Typography>

          {!hasMore && logs.length < totalCount && (
            <Typography variant="body2" color="textSecondary">
              Reached end of results
            </Typography>
          )}
        </Box>
      )}
    </Paper>
  );
};
