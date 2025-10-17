import {
  CatalogProcessor,
  CatalogProcessorEmit,
  processingResult,
} from '@backstage/plugin-catalog-node';
import { LocationSpec } from '@backstage/plugin-catalog-common';
import { 
  RELATION_OWNED_BY,
  RELATION_PART_OF,
} from '@backstage/catalog-model';
import { EnvironmentEntityV1alpha1 } from '../kinds/EnvironmentEntityV1alpha1';

/**
 * Processor for Environment entities
 */
export class EnvironmentEntityProcessor implements CatalogProcessor {
  getProcessorName(): string {
    return 'EnvironmentEntityProcessor';
  }

  async validateEntityKind(entity: EnvironmentEntityV1alpha1): Promise<boolean> {
    return entity.kind === 'Environment';
  }

  async postProcessEntity(
    entity: EnvironmentEntityV1alpha1,
    _location: LocationSpec,
    emit: CatalogProcessorEmit,
  ): Promise<EnvironmentEntityV1alpha1> {
    // Validate required fields
    if (entity.kind === 'Environment') {
      if (!entity.spec?.type) {
        throw new Error('Environment entity must have spec.type');
      }
      if (!entity.spec?.owner) {
        throw new Error('Environment entity must have spec.owner');
      }

      // Emit relationships based on spec fields
      const sourceRef = {
        kind: entity.kind.toLowerCase(),
        namespace: entity.metadata.namespace || 'default',
        name: entity.metadata.name,
      };

      // Emit partOf relationship to domain
      if (entity.spec.domain) {
        emit(processingResult.relation({
          source: sourceRef,
          target: { kind: 'domain', namespace: 'default', name: entity.spec.domain },
          type: RELATION_PART_OF,
        }));
      }

      // Emit ownedBy relationship to owner
      if (entity.spec.owner) {
        emit(processingResult.relation({
          source: sourceRef,
          target: { kind: 'group', namespace: 'default', name: entity.spec.owner },
          type: RELATION_OWNED_BY,
        }));
      }
    }

    return entity;
  }

  async preProcessEntity(
    entity: EnvironmentEntityV1alpha1,
    _location: LocationSpec,
    _emit: CatalogProcessorEmit,
  ): Promise<EnvironmentEntityV1alpha1> {
    // Set default values if needed
    if (entity.kind === 'Environment' && entity.spec) {
      // Set default isProduction if not specified
      if (entity.spec.isProduction === undefined) {
        entity.spec.isProduction = entity.spec.type === 'production';
      }
    }

    return entity;
  }

  async processEntity(
    entity: EnvironmentEntityV1alpha1,
    location: LocationSpec,
    emit: CatalogProcessorEmit,
  ): Promise<EnvironmentEntityV1alpha1> {
    // Only process Environment entities
    if (entity.kind !== 'Environment') {
      return entity;
    }

    // Emit the processed entity
    emit(processingResult.entity(location, entity));
    
    return entity;
  }
}
