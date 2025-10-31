import { TableColumn } from '@backstage/core-components';
import {
  CatalogTable,
  CatalogTableRow,
  CatalogTableColumnsFunc,
} from '@backstage/plugin-catalog';
import { EntityRefLinks } from '@backstage/plugin-catalog-react';

/**
 * Custom column factory for creating a "Project" column (replaces "System")
 */
function createProjectColumn(): TableColumn<CatalogTableRow> {
  return {
    title: 'System (Project)',
    field: 'resolved.partOfSystemRelationTitle',
    customFilterAndSearch: (query, row) => {
      if (!row.resolved.partOfSystemRelations) {
        return false;
      }

      const systemNames = row.resolved.partOfSystemRelations.map(
        ref => ref.name,
      );

      const searchText = systemNames.join(', ').toLocaleUpperCase('en-US');
      return searchText.includes(query.toLocaleUpperCase('en-US'));
    },
    render: ({ resolved }) => (
      <EntityRefLinks
        entityRefs={resolved.partOfSystemRelations}
        defaultKind="system"
      />
    ),
  };
}

/**
 * Custom catalog table columns function with Choreo naming:
 * - System → Project
 * - Domain → Organization (handled by entity kind picker)
 */
export const choreoCatalogTableColumns: CatalogTableColumnsFunc =
  entityListContext => {
    // Get the default columns
    const defaultColumns = CatalogTable.defaultColumnsFunc(entityListContext);

    // Replace the "System" column with "Project" column
    return defaultColumns.map(column => {
      // Check if this is the system column by checking the field property
      if (
        typeof column === 'object' &&
        'field' in column &&
        column.field === 'resolved.partOfSystemRelationTitle'
      ) {
        return createProjectColumn();
      }
      return column;
    });
  };
