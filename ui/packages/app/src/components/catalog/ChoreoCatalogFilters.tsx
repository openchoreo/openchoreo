import {
  EntityLifecyclePicker,
  EntityNamespacePicker,
  EntityOwnerPicker,
  EntityOwnerPickerProps,
  EntityProcessingStatusPicker,
  EntityTagPicker,
  EntityTypePicker,
  UserListPicker,
  UserListFilterKind,
} from '@backstage/plugin-catalog-react';
import { ChoreoEntityKindPicker } from './ChoreoEntityKindPicker';

/**
 * Props for choreo catalog filters.
 *
 * @public
 */
export type ChoreoCatalogFiltersProps = {
  initialKind?: string;
  initiallySelectedFilter?: UserListFilterKind;
  ownerPickerMode?: EntityOwnerPickerProps['mode'];
  initiallySelectedNamespaces?: string[];
};

/**
 * Choreo Catalog Filters with Choreo entity kind names
 * - Domain is displayed as "Organization"
 * - System is displayed as "Project"
 *
 * @public
 */
export const ChoreoCatalogFilters = (props: ChoreoCatalogFiltersProps) => {
  const {
    initialKind,
    initiallySelectedFilter,
    ownerPickerMode,
    initiallySelectedNamespaces,
  } = props;
  return (
    <>
      <ChoreoEntityKindPicker initialFilter={initialKind} />
      <EntityTypePicker />
      <UserListPicker initialFilter={initiallySelectedFilter} />
      <EntityOwnerPicker mode={ownerPickerMode} />
      <EntityLifecyclePicker />
      <EntityTagPicker />
      <EntityProcessingStatusPicker />
      <EntityNamespacePicker
        initiallySelectedNamespaces={initiallySelectedNamespaces}
      />
    </>
  );
};
