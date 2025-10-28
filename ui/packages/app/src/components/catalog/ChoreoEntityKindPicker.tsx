import { Select } from '@backstage/core-components';
import { alertApiRef, useApi } from '@backstage/core-plugin-api';
import Box from '@material-ui/core/Box';
import { useEffect, useMemo, useState } from 'react';
import {
  EntityKindFilter,
  useEntityList,
  catalogApiRef,
} from '@backstage/plugin-catalog-react';

// Mapping of internal kind names to Choreo entity kind names
const kindDisplayNames: Record<string, string> = {
  domain: 'Domain (Organization)',
  system: 'System (Project)',
  component: 'Component',
  api: 'API',
  user: 'User',
  group: 'Group',
  resource: 'Resource',
  location: 'Location',
  template: 'Template',
};

// Custom order for displaying entity kinds in the dropdown
// Organization first, then Project, then Component, then others alphabetically
const kindDisplayOrder: string[] = [
  'domain',
  'system',
  'component',
  'api',
  'resource',
  'user',
  'group',
  'location',
  'template',
];

// Hook to fetch all available Choreo entity kinds from the catalog
function useAllKinds(): {
  allKinds: Map<string, string>;
  loading: boolean;
  error?: Error;
} {
  const catalogApi = useApi(catalogApiRef);
  const [allKinds, setAllKinds] = useState<Map<string, string>>(new Map());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | undefined>();

  useEffect(() => {
    let isMounted = true;

    const fetchKinds = async () => {
      try {
        setLoading(true);
        // Fetch all entities to get available kinds
        const { items } = await catalogApi.getEntities({
          fields: ['kind'],
        });

        if (!isMounted) return;

        // Extract unique kinds
        const kindsSet = new Set<string>();
        items.forEach(entity => {
          if (entity.kind) {
            kindsSet.add(entity.kind);
          }
        });

        // Create map with kinds as both key and value (we'll override the value with custom label later)
        const kindsMap = new Map<string, string>();
        kindsSet.forEach(kind => {
          kindsMap.set(kind, kind);
        });

        setAllKinds(kindsMap);
        setLoading(false);
      } catch (err) {
        if (!isMounted) return;
        setError(err as Error);
        setLoading(false);
      }
    };

    fetchKinds();

    return () => {
      isMounted = false;
    };
  }, [catalogApi]);

  return { allKinds, loading, error };
}

function useEntityKindFilter(opts: { initialFilter: string }): {
  loading: boolean;
  error?: Error;
  allKinds: Map<string, string>;
  selectedKind: string;
  setSelectedKind: (kind: string) => void;
} {
  const {
    filters,
    queryParameters: { kind: kindParameter },
    updateFilters,
  } = useEntityList();

  const queryParamKind = useMemo(
    () => [kindParameter].flat()[0],
    [kindParameter],
  );

  const [selectedKind, setSelectedKind] = useState(
    queryParamKind ?? filters.kind?.value ?? opts.initialFilter,
  );

  // Set selected kinds on query parameter updates
  useEffect(() => {
    if (queryParamKind) {
      setSelectedKind(queryParamKind);
    }
  }, [queryParamKind]);

  // Set selected kind from filters
  useEffect(() => {
    if (filters.kind?.value) {
      setSelectedKind(filters.kind?.value);
    }
  }, [filters.kind]);

  const { allKinds, loading, error } = useAllKinds();

  // Override the label with our custom display name
  const selectedKindLabel =
    kindDisplayNames[selectedKind.toLowerCase()] ||
    allKinds.get(selectedKind) ||
    selectedKind;

  useEffect(() => {
    updateFilters({
      kind: selectedKind
        ? new EntityKindFilter(selectedKind, selectedKindLabel)
        : undefined,
    });
  }, [selectedKind, selectedKindLabel, updateFilters]);

  return {
    loading,
    error,
    allKinds,
    selectedKind,
    setSelectedKind,
  };
}

/**
 * Custom EntityKindPicker that displays Choreo names for entity kinds
 * Maps: Domain -> Org, System -> Project
 */
export interface ChoreoEntityKindPickerProps {
  allowedKinds?: string[];
  initialFilter?: string;
  hidden?: boolean;
}

export const ChoreoEntityKindPicker = (props: ChoreoEntityKindPickerProps) => {
  const { allowedKinds, hidden, initialFilter = 'component' } = props;

  const alertApi = useApi(alertApiRef);

  const { error, allKinds, selectedKind, setSelectedKind } =
    useEntityKindFilter({
      initialFilter: initialFilter,
    });

  useEffect(() => {
    if (error) {
      alertApi.post({
        message: 'Failed to load entity kinds',
        severity: 'error',
      });
    }
  }, [error, alertApi]);

  if (error) return null;

  // Create a new map with custom labels
  const customKindsMap = new Map<string, string>();
  allKinds.forEach((value, key) => {
    const lowerKey = key.toLowerCase();
    const customLabel = kindDisplayNames[lowerKey] || value;
    customKindsMap.set(key, customLabel);
  });

  // Filter kinds if allowedKinds is specified
  const filteredKinds = allowedKinds
    ? new Map(
        [...customKindsMap].filter(([key]) =>
          allowedKinds.some(
            allowed => allowed.toLowerCase() === key.toLowerCase(),
          ),
        ),
      )
    : customKindsMap;

  // Sort items according to kindDisplayOrder
  const items = [...filteredKinds.entries()]
    .sort(([keyA], [keyB]) => {
      const indexA = kindDisplayOrder.indexOf(keyA.toLowerCase());
      const indexB = kindDisplayOrder.indexOf(keyB.toLowerCase());

      // If both are in the order list, sort by their position
      if (indexA !== -1 && indexB !== -1) {
        return indexA - indexB;
      }
      // If only A is in the order list, A comes first
      if (indexA !== -1) return -1;
      // If only B is in the order list, B comes first
      if (indexB !== -1) return 1;
      // If neither is in the order list, sort alphabetically by label
      return keyA.localeCompare(keyB);
    })
    .map(([key, value]) => ({
      label: value,
      value: key.toLowerCase(), // Ensure value is lowercase to match selectedKind
    }));

  return hidden ? null : (
    <Box pb={1} pt={1}>
      <Select
        label="Kind"
        items={items}
        selected={selectedKind.toLowerCase()}
        onChange={value => setSelectedKind(String(value))}
      />
    </Box>
  );
};
