import React, { useState, useEffect } from 'react';
import {
  Drawer,
  Typography,
  IconButton,
  Box,
  Divider,
  CircularProgress,
} from '@material-ui/core';
import { Close } from '@material-ui/icons';
import {
  useApi,
  discoveryApiRef,
  identityApiRef,
} from '@backstage/core-plugin-api';
import type {
  ModelsBuild,
  LogEntry,
} from '@internal/plugin-openchoreo-api';
import { fetchBuildLogsForBuild } from '../../api/buildLogs';

interface BuildLogsProps {
  open: boolean;
  onClose: () => void;
  build: ModelsBuild | null;
}

export const BuildLogs = ({ open, onClose, build }: BuildLogsProps) => {
  const discoveryApi = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchBuildLogs = async (selectedBuild: ModelsBuild) => {
    setLoading(true);
    setError(null);
    setLogs([]);

    try {
      const logsData = await fetchBuildLogsForBuild(
        discoveryApi,
        identityApi,
        selectedBuild,
      );
      setLogs(logsData.logs || []);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to fetch build logs',
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (open && build) {
      fetchBuildLogs(build);
    }
  }, [open, build]);

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        style: {
          width: '600px',
          maxWidth: '80vw',
        },
      }}
    >
      <Box p={2}>
        <Box
          display="flex"
          justifyContent="space-between"
          alignItems="center"
          mb={2}
        >
          <Typography variant="h6">
            Build Logs - {build?.name || 'Unknown Build'}
          </Typography>
          <IconButton onClick={onClose} size="small">
            <Close />
          </IconButton>
        </Box>

        <Divider />

        <Box mt={2}>
          {build ? (
            <Box>
              <Typography variant="body2" color="textSecondary" gutterBottom>
                Build Name: {build.name}
              </Typography>
              <Typography variant="body2" color="textSecondary" gutterBottom>
                Status: {build.status}
              </Typography>
              <Typography variant="body2" color="textSecondary" gutterBottom>
                Commit: {build.commit?.substring(0, 8) || 'N/A'}
              </Typography>
              <Typography variant="body2" color="textSecondary" gutterBottom>
                Created: {new Date(build.createdAt).toLocaleString()}
              </Typography>

              <Box mt={3}>
                <Typography variant="subtitle2" gutterBottom>
                  Logs:
                </Typography>
                <Box
                  p={2}
                  style={{
                    backgroundColor: '#f5f5f5',
                    fontFamily: 'monospace',
                    fontSize: '12px',
                    height: '400px',
                    overflow: 'auto',
                    border: '1px solid #e0e0e0',
                    borderRadius: '4px',
                    whiteSpace: 'pre-wrap',
                  }}
                >
                  {loading ? (
                    <Box
                      display="flex"
                      justifyContent="center"
                      alignItems="center"
                      height="100%"
                    >
                      <CircularProgress size={24} />
                      <Typography variant="body2" style={{ marginLeft: '8px' }}>
                        Loading logs...
                      </Typography>
                    </Box>
                  ) : error ? (
                    <Typography variant="body2" color="error">
                      Error: {error}
                    </Typography>
                  ) : logs.length > 0 ? (
                    logs.map((logEntry, index) => (
                      <Box key={index} style={{ marginBottom: '4px' }}>
                        <Typography
                          variant="body2"
                          style={{ fontSize: '11px', color: '#666' }}
                        >
                          [{new Date(logEntry.timestamp).toLocaleTimeString()}]
                        </Typography>
                        <Typography
                          variant="body2"
                          style={{ fontSize: '12px' }}
                        >
                          {logEntry.log}
                        </Typography>
                      </Box>
                    ))
                  ) : (
                    <Typography variant="body2">
                      No logs available for this build
                    </Typography>
                  )}
                </Box>
              </Box>
            </Box>
          ) : (
            <Typography variant="body1">No build selected</Typography>
          )}
        </Box>
      </Box>
    </Drawer>
  );
};
