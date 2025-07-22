import React, { useState } from 'react';
import {
  TableRow,
  TableCell,
  Typography,
  Chip,
  Box,
  Collapse,
  IconButton,
  Tooltip,
} from '@material-ui/core';
import { makeStyles } from '@material-ui/core/styles';
import ExpandMore from '@material-ui/icons/ExpandMore';
import ExpandLess from '@material-ui/icons/ExpandLess';
import FileCopy from '@material-ui/icons/FileCopy';
import { LogEntry as LogEntryType } from './types';

const useStyles = makeStyles(theme => ({
  logRow: {
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
    cursor: 'pointer',
  },
  expandedRow: {
    backgroundColor: theme.palette.action.selected,
  },
  timestampCell: {
    fontFamily: 'monospace',
    fontSize: '0.85rem',
    whiteSpace: 'nowrap',
    width: '140px',
  },
  logLevelChip: {
    fontSize: '0.75rem',
    fontWeight: 'bold',
    minWidth: '60px',
  },
  errorChip: {
    backgroundColor: theme.palette.error.main,
    color: theme.palette.error.contrastText,
  },
  warnChip: {
    backgroundColor: theme.palette.warning.main,
    color: theme.palette.warning.contrastText,
  },
  infoChip: {
    backgroundColor: theme.palette.info.main,
    color: theme.palette.info.contrastText,
  },
  debugChip: {
    backgroundColor: theme.palette.grey[400],
    color: theme.palette.getContrastText(theme.palette.grey[400]),
  },
  logMessage: {
    fontFamily: 'monospace',
    fontSize: '0.875rem',
    wordBreak: 'break-word',
    maxWidth: '400px',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  expandedLogMessage: {
    whiteSpace: 'pre-wrap',
    maxWidth: 'none',
    overflow: 'visible',
  },
  containerCell: {
    fontSize: '0.875rem',
    color: theme.palette.text.secondary,
  },
  podCell: {
    fontSize: '0.875rem',
    color: theme.palette.text.secondary,
    fontFamily: 'monospace',
  },
  expandButton: {
    padding: theme.spacing(0.5),
  },
  expandedContent: {
    padding: theme.spacing(2),
    backgroundColor: theme.palette.grey[50],
    borderRadius: theme.shape.borderRadius,
  },
  metadataSection: {
    marginTop: theme.spacing(2),
  },
  metadataTitle: {
    fontWeight: 'bold',
    marginBottom: theme.spacing(1),
  },
  metadataItem: {
    display: 'flex',
    marginBottom: theme.spacing(0.5),
  },
  metadataKey: {
    fontWeight: 'bold',
    minWidth: '120px',
    marginRight: theme.spacing(1),
  },
  metadataValue: {
    fontFamily: 'monospace',
    fontSize: '0.875rem',
    color: theme.palette.text.secondary,
  },
  copyButton: {
    padding: theme.spacing(0.5),
    marginLeft: theme.spacing(1),
  },
  fullLogMessage: {
    fontFamily: 'monospace',
    fontSize: '0.875rem',
    whiteSpace: 'pre-wrap',
    backgroundColor: theme.palette.grey[100],
    padding: theme.spacing(1),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.grey[300]}`,
    maxHeight: '200px',
    overflow: 'auto',
  },
}));

interface LogEntryProps {
  log: LogEntryType;
}

export const LogEntry: React.FC<LogEntryProps> = ({ log }) => {
  const classes = useStyles();
  const [expanded, setExpanded] = useState(false);

  const getLogLevelChipClass = (level: string) => {
    switch (level) {
      case 'ERROR':
        return classes.errorChip;
      case 'WARN':
        return classes.warnChip;
      case 'INFO':
        return classes.infoChip;
      case 'DEBUG':
        return classes.debugChip;
      default:
        return '';
    }
  };

  const formatTimestamp = (timestamp: string) => {
    return new Date(timestamp).toLocaleString();
  };

  const truncatePodId = (podId: string) => {
    return podId.length > 8 ? `${podId.substring(0, 8)}...` : podId;
  };

  const handleCopyLog = (event: React.MouseEvent) => {
    event.stopPropagation();
    navigator.clipboard.writeText(log.log).catch(error => {
      console.error('Failed to copy log to clipboard:', error);
    });
  };

  const handleRowClick = () => {
    setExpanded(!expanded);
  };

  return (
    <>
      <TableRow
        className={`${classes.logRow} ${expanded ? classes.expandedRow : ''}`}
        onClick={handleRowClick}
      >
        <TableCell className={classes.timestampCell}>
          {formatTimestamp(log.timestamp)}
        </TableCell>
        <TableCell>
          <Chip
            label={log.logLevel}
            size="small"
            className={`${classes.logLevelChip} ${getLogLevelChipClass(
              log.logLevel,
            )}`}
          />
        </TableCell>
        <TableCell>
          <Box display="flex" alignItems="center">
            <Typography
              className={`${classes.logMessage} ${
                expanded ? classes.expandedLogMessage : ''
              }`}
            >
              {log.log}
            </Typography>
            <Tooltip title="Copy log message">
              <IconButton
                className={classes.copyButton}
                onClick={handleCopyLog}
                size="small"
              >
                <FileCopy fontSize="small" />
              </IconButton>
            </Tooltip>
          </Box>
        </TableCell>
        <TableCell className={classes.containerCell}>
          {log.containerName}
        </TableCell>
        <TableCell className={classes.podCell}>
          <Tooltip title={log.podId}>
            <span>{truncatePodId(log.podId)}</span>
          </Tooltip>
        </TableCell>
        <TableCell>
          <IconButton
            className={classes.expandButton}
            size="small"
            onClick={e => {
              e.stopPropagation();
              setExpanded(!expanded);
            }}
          >
            {expanded ? <ExpandLess /> : <ExpandMore />}
          </IconButton>
        </TableCell>
      </TableRow>

      {expanded && (
        <TableRow>
          <TableCell colSpan={6} style={{ paddingBottom: 0, paddingTop: 0 }}>
            <Collapse in={expanded} timeout="auto" unmountOnExit>
              <Box className={classes.expandedContent}>
                <Typography variant="h6" gutterBottom>
                  Full Log Message
                </Typography>
                <Box className={classes.fullLogMessage}>{log.log}</Box>

                <Box className={classes.metadataSection}>
                  <Typography variant="h6" className={classes.metadataTitle}>
                    Metadata
                  </Typography>

                  <Box className={classes.metadataItem}>
                    <span className={classes.metadataKey}>Component:</span>
                    <span className={classes.metadataValue}>
                      {log.componentId}
                    </span>
                  </Box>

                  <Box className={classes.metadataItem}>
                    <span className={classes.metadataKey}>Environment:</span>
                    <span className={classes.metadataValue}>
                      {log.environmentId}
                    </span>
                  </Box>

                  <Box className={classes.metadataItem}>
                    <span className={classes.metadataKey}>Project:</span>
                    <span className={classes.metadataValue}>
                      {log.projectId}
                    </span>
                  </Box>

                  <Box className={classes.metadataItem}>
                    <span className={classes.metadataKey}>Namespace:</span>
                    <span className={classes.metadataValue}>
                      {log.namespace}
                    </span>
                  </Box>

                  <Box className={classes.metadataItem}>
                    <span className={classes.metadataKey}>Pod ID:</span>
                    <span className={classes.metadataValue}>{log.podId}</span>
                  </Box>

                  <Box className={classes.metadataItem}>
                    <span className={classes.metadataKey}>Container:</span>
                    <span className={classes.metadataValue}>
                      {log.containerName}
                    </span>
                  </Box>

                  {log.version && (
                    <Box className={classes.metadataItem}>
                      <span className={classes.metadataKey}>Version:</span>
                      <span className={classes.metadataValue}>
                        {log.version}
                      </span>
                    </Box>
                  )}

                  {Object.keys(log.labels).length > 0 && (
                    <>
                      <Typography
                        variant="subtitle1"
                        className={classes.metadataTitle}
                        style={{ marginTop: 16 }}
                      >
                        Labels
                      </Typography>
                      {Object.entries(log.labels).map(([key, value]) => (
                        <Box key={key} className={classes.metadataItem}>
                          <span className={classes.metadataKey}>{key}:</span>
                          <span className={classes.metadataValue}>{value}</span>
                        </Box>
                      ))}
                    </>
                  )}
                </Box>
              </Box>
            </Collapse>
          </TableCell>
        </TableRow>
      )}
    </>
  );
};
