import { lazy, Suspense, useEffect, useState } from 'react';
import {
  Content,
  Header,
  HeaderLabel,
  Page,
  Progress,
} from '@backstage/core-components';

import { useEntity } from '@backstage/plugin-catalog-react';
import {
  discoveryApiRef,
  identityApiRef,
  useApi,
} from '@backstage/core-plugin-api';
import { getCellDiagramInfo } from '../../api/getCellDiagramInfo';
import { Project } from '@wso2/cell-diagram';

const CellView = lazy(() =>
  import('@wso2/cell-diagram').then(module => ({
    default: module.CellDiagram,
  })),
);

export const CellDiagram = () => {
  const { entity } = useEntity();
  const [cellDiagramData, setCellDiagramData] = useState<Project>();
  const discovery = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const data = await getCellDiagramInfo(entity, discovery, identityApi);
        setCellDiagramData(data as Project);
      } catch (error) {}
    };

    fetchData();
  }, [entity, discovery, identityApi]);

  return (
    <Page themeId="tool">
      <Header title="Architecture Diagram" type="tool">
        <HeaderLabel label="Project" value={entity.metadata.name} />
      </Header> 
      <Content>
        <CellView project={cellDiagramData} />
        <Suspense fallback={<Progress />} />
      </Content>
    </Page>
  );
};
